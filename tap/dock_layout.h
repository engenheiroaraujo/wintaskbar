// Acompanha uma Dock na thread XAML. Os callbacks possuem o controlador, mas o
// controlador só guarda referência fraca ao elemento: não há ciclo COM.
#pragma once
#include "dock_geometry.h"
#include <windows.h>
#include <functional>
#include <memory>
#include <winrt/Windows.UI.Core.h>
#include <winrt/Windows.UI.Xaml.h>
#include <winrt/Windows.UI.Xaml.Media.h>

namespace tws {
namespace dx = winrt::Windows::UI::Xaml;

class DockLayout : public std::enable_shared_from_this<DockLayout> {
public:
    using LogFn = std::function<void(std::wstring const&)>;

    static std::shared_ptr<DockLayout> Attach(dx::FrameworkElement const& frame, double gap, LogFn log) {
        auto self = std::shared_ptr<DockLayout>(new DockLayout(frame, gap, std::move(log)));
        try {
            self->m_layout = frame.LayoutUpdated([self](auto const&, auto const&) { self->Queue(); });
            self->m_hasLayout = true;
            self->m_unloaded = frame.Unloaded([self](auto const&, auto const&) { self->Detach(); });
            self->m_hasUnloaded = true;
            self->Queue();
        } catch (...) {
            self->Detach();
            throw;
        }
        return self;
    }

    // Remove/Stop podem vir da thread de diagnóstico. Toda operação XAML volta
    // ao dispatcher dono; nenhum lock é mantido enquanto ela é executada.
    void Stop() noexcept {
        try {
            auto self = shared_from_this();
            if (m_dispatcher.HasThreadAccess()) Detach();
            else m_dispatcher.RunAsync(winrt::Windows::UI::Core::CoreDispatcherPriority::Normal,
                                      [self] { self->Detach(); });
        } catch (...) {
            // Um dispatcher encerrado não executará novos callbacks.
        }
    }

private:
    DockLayout(dx::FrameworkElement const& frame, double gap, LogFn log)
        : m_frame(frame), m_dispatcher(frame.Dispatcher()), m_gap(gap), m_log(std::move(log)) {}

    void Queue() noexcept {
        if (m_stopped || m_pending) return;
        try {
            m_pending = true;
            std::weak_ptr<DockLayout> weak = shared_from_this();
            m_dispatcher.RunAsync(winrt::Windows::UI::Core::CoreDispatcherPriority::Low, [weak] {
                if (auto self = weak.lock()) {
                    self->m_pending = false;
                    if (!self->m_stopped) self->Update();
                }
            });
        } catch (...) { m_pending = false; Report(L"dispatcher unavailable"); }
    }

    struct NativeBounds {
        double width = 0, trayLeft = 0;
        unsigned matches = 0;
        DWORD thread = 0;
        double expectedWidth = 0, expectedHeight = 0;
    };

    static BOOL CALLBACK FindBar(HWND window, LPARAM parameter) {
        auto& b = *reinterpret_cast<NativeBounds*>(parameter);
        wchar_t name[64]{};
        GetClassNameW(window, name, 64);
        if (wcscmp(name, L"Shell_TrayWnd") && wcscmp(name, L"Shell_SecondaryTrayWnd")) return TRUE;
        if (GetWindowThreadProcessId(window, nullptr) != b.thread) return TRUE;
        RECT client{};
        if (!GetClientRect(window, &client)) return TRUE;
        if (std::abs(double(client.right) - b.expectedWidth) > 2 ||
            std::abs(double(client.bottom) - b.expectedHeight) > 2) return TRUE;
        HWND tray = FindWindowExW(window, nullptr, L"TrayNotifyWnd", nullptr);
        RECT trayRect{};
        POINT origin{};
        if (!tray || !GetWindowRect(tray, &trayRect) || !ClientToScreen(window, &origin)) return TRUE;
        b.width = client.right;
        b.trayLeft = trayRect.left - origin.x;
        ++b.matches;
        return TRUE;
    }

    static NativeBounds Measure(dx::XamlRoot const& root) {
        auto size = root.Size();
        double scale = root.RasterizationScale();
        NativeBounds bounds;
        bounds.thread = GetCurrentThreadId();
        bounds.expectedWidth = size.Width * scale;
        bounds.expectedHeight = size.Height * scale;
        // Os retângulos Win32 são físicos apenas durante esta consulta. O
        // contexto original da thread é restaurado antes de qualquer chamada XAML.
        auto previous = SetThreadDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
        if (!previous) return bounds;
        EnumThreadWindows(bounds.thread, FindBar, reinterpret_cast<LPARAM>(&bounds));
        SetThreadDpiAwarenessContext(previous);
        if (scale > 0) { bounds.width /= scale; bounds.trayLeft /= scale; }
        return bounds;
    }

    void Update() noexcept {
        if (m_updating) return;
        m_updating = true;
        try {
            auto frame = m_frame.get();
            if (!frame || !frame.IsLoaded()) { m_updating = false; return; }
            auto root = frame.XamlRoot();
            if (!root || !root.Content()) { m_updating = false; return; }
            if (m_root.get() != root) {
                if (auto old = m_root.get(); old && m_hasRoot) old.Changed(m_rootChanged);
                m_hasRoot = false;
                m_root = root;
                auto self = shared_from_this();
                m_rootChanged = root.Changed([self](auto const&, auto const&) { self->Queue(); });
                m_hasRoot = true;
            }
            auto bounds = Measure(root);
            if (bounds.matches != 1 || bounds.trayLeft <= 0 || bounds.trayLeft > bounds.width) {
                // Não adivinhar a barra em outra thread/monitor ou árvore XAML.
                Restore(frame);
                Report(L"taskbar/tray geometry unavailable or ambiguous");
                m_updating = false;
                return;
            }
            if (!m_initialized) {
                m_originalMax = frame.MaxWidth();
                m_originalMin = frame.MinWidth();
                m_maxLocal = frame.ReadLocalValue(dx::FrameworkElement::MaxWidthProperty());
                m_minLocal = frame.ReadLocalValue(dx::FrameworkElement::MinWidthProperty());
                m_transformLocal = frame.ReadLocalValue(dx::UIElement::RenderTransformProperty());
                m_originalTransform = frame.RenderTransform();
                // O Theme padrão não transforma o quadro. Uma transformação
                // customizada não deve ser sobrescrita por esta política.
                if (auto transform = m_originalTransform) {
                    auto origin = transform.TransformPoint({0, 0});
                    auto x = transform.TransformPoint({1, 0});
                    auto y = transform.TransformPoint({0, 1});
                    if (origin.X != 0 || origin.Y != 0 || x.X != 1 || x.Y != 0 || y.X != 0 || y.Y != 1) {
                        Report(L"DockLayoutGap requires an untransformed frame");
                        m_updating = false;
                        return;
                    }
                }
                m_shift = dx::Media::TranslateTransform();
                m_initialized = true;
                m_barWidth = m_trayLeft = m_measuredWidth = -1;
                frame.RenderTransform(m_shift);
            }
            if (frame.RenderTransform() != m_shift) {
                Report(L"frame transform changed; adaptive layout detached");
                Detach();
                m_updating = false;
                return;
            }
            auto point = frame.TransformToVisual(root.Content()).TransformPoint({0, 0});
            if (bounds.width != m_barWidth || bounds.trayLeft != m_trayLeft) {
                m_barWidth = bounds.width;
                m_trayLeft = bounds.trayLeft;
                m_measuredWidth = -1;
                m_widthLimit = std::min(m_originalMax, bounds.width);
                if (std::abs(frame.MaxWidth() - m_widthLimit) > 0.01) {
                    frame.MaxWidth(m_widthLimit);
                    // Aguardar a medição nativa com o novo espaço disponível.
                    m_updating = false;
                    return;
                }
            }
            double measuredWidth = frame.ActualWidth();
            auto fit = FitDock(bounds.width, bounds.trayLeft, m_gap, frame.ActualWidth(),
                               double(point.X) - m_shift.X(), m_widthLimit);
            // Não reduzir duas vezes com a mesma medição ainda pendente. Se o
            // Windows não diminuir mais, o controlador não gira num laço.
            if (measuredWidth != m_measuredWidth) m_widthLimit = fit.maxWidth;
            m_measuredWidth = measuredWidth;
            if (std::abs(frame.MinWidth() - std::min(m_originalMin, m_widthLimit)) > 0.01)
                frame.MinWidth(std::min(m_originalMin, m_widthLimit));
            if (std::abs(frame.MaxWidth() - m_widthLimit) > 0.01) frame.MaxWidth(m_widthLimit);
            if (std::abs(m_shift.X() - fit.offset) > 0.01) m_shift.X(fit.offset);
        } catch (...) { Report(L"layout update failed"); }
        m_updating = false;
    }

    static void RestoreProperty(dx::DependencyObject const& object, dx::DependencyProperty const& property,
                                winrt::Windows::Foundation::IInspectable const& value) {
        if (value == dx::DependencyProperty::UnsetValue()) object.ClearValue(property);
        else object.SetValue(property, value);
    }

    void Restore(dx::FrameworkElement const& frame) noexcept {
        if (!m_initialized) return;
        try {
            RestoreProperty(frame, dx::FrameworkElement::MaxWidthProperty(), m_maxLocal);
            RestoreProperty(frame, dx::FrameworkElement::MinWidthProperty(), m_minLocal);
            if (frame.RenderTransform() == m_shift)
                RestoreProperty(frame, dx::UIElement::RenderTransformProperty(), m_transformLocal);
        } catch (...) { Report(L"layout restore failed"); }
        m_initialized = false;
        m_shift = nullptr;
        m_originalTransform = nullptr;
        m_maxLocal = m_minLocal = m_transformLocal = nullptr;
    }

    void Detach() noexcept {
        if (m_stopped) return;
        m_stopped = true;
        try {
            if (auto root = m_root.get(); root && m_hasRoot) root.Changed(m_rootChanged);
        } catch (...) {}
        try {
            if (auto frame = m_frame.get()) {
                try { if (m_hasLayout) frame.LayoutUpdated(m_layout); } catch (...) {}
                try { if (m_hasUnloaded) frame.Unloaded(m_unloaded); } catch (...) {}
                Restore(frame);
            }
        } catch (...) {}
    }

    void Report(wchar_t const* reason) noexcept {
        if (m_reported) return;
        m_reported = true;
        try { m_log(std::wstring(L"dock layout: ") + reason); } catch (...) {}
    }

    winrt::weak_ref<dx::FrameworkElement> m_frame;
    winrt::weak_ref<dx::XamlRoot> m_root;
    winrt::Windows::UI::Core::CoreDispatcher m_dispatcher{nullptr};
    dx::Media::TranslateTransform m_shift{nullptr};
    dx::Media::Transform m_originalTransform{nullptr};
    winrt::Windows::Foundation::IInspectable m_maxLocal{nullptr}, m_minLocal{nullptr}, m_transformLocal{nullptr};
    winrt::event_token m_layout{}, m_unloaded{}, m_rootChanged{};
    double m_gap, m_originalMax = 0, m_originalMin = 0;
    double m_barWidth = -1, m_trayLeft = -1, m_measuredWidth = -1, m_widthLimit = 0;
    LogFn m_log;
    bool m_pending = false, m_updating = false, m_stopped = false, m_initialized = false;
    bool m_hasLayout = false, m_hasUnloaded = false, m_reported = false;
    bool m_hasRoot = false;
};
}
