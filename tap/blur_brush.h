// BlurBrush: um pincel XAML que desfoca o que está atrás do elemento e coloca uma
// tinta por cima (reduzido a desfoque + tinta).
//
// Grafo de efeitos:   backdrop -> GaussianBlur ----\
//                                               >-- Composite(SourceOver) -> brush
//                              Flood(tint) ----/

#pragma once

#include "effects.h"

#include <algorithm>
#include <atomic>
#include <string>

#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.UI.Composition.h>
#include <winrt/Windows.UI.ViewManagement.h>
#include <winrt/Windows.UI.Xaml.h>
#include <winrt/Windows.UI.Xaml.Hosting.h>
#include <winrt/Windows.UI.Xaml.Markup.h>
#include <winrt/Windows.UI.Xaml.Media.h>

namespace tws
{
namespace wuc = winrt::Windows::UI::Composition;
namespace wux = winrt::Windows::UI::Xaml;
namespace wuxm = winrt::Windows::UI::Xaml::Media;

class BlurBrush : public wuxm::XamlCompositionBrushBaseT<BlurBrush>
{
public:
    // themeColorKey: nome de um recurso de cor do tema (ex.: "SystemChromeDarkColor")
    // usado como tinta, com `tintAlpha` como alfa. fallback é usado quando a
    // chave não pode ser resolvida e quando os efeitos avançados estão desativados.
    BlurBrush(wux::UIElement const& element, float blurAmount, std::wstring themeColorKey, uint8_t tintAlpha,
              winrt::Windows::UI::Color fallback)
        : m_compositor(wux::Hosting::ElementCompositionPreview::GetElementVisual(element).Compositor()),
          m_blurAmount(blurAmount),
          m_tint(fallback),
          m_tintAlpha(tintAlpha),
          m_fallback(fallback)
    {
        auto fe = element.try_as<wux::FrameworkElement>();
        if (!fe || themeColorKey.empty()) {
            return;
        }

        // Um SolidColorBrush que liga a cor do tema; inserido nos
        // recursos do elemento para que a busca do tema tenha uma árvore onde resolver.
        try {
            std::wstring xaml =
                L"<SolidColorBrush xmlns='http://schemas.microsoft.com/winfx/2006/xaml/presentation' "
                L"Color='{ThemeResource " +
                themeColorKey + L"}'/>";
            auto proxy = wux::Markup::XamlReader::Load(xaml).try_as<wuxm::SolidColorBrush>();
            if (proxy) {
                static std::atomic<uint32_t> counter{0};
                m_proxyKey = L"__WinTaskbarTint_" + std::to_wstring(++counter);
                fe.Resources().Insert(winrt::box_value(m_proxyKey), proxy);
                m_proxy = proxy;
                m_owner = winrt::make_weak(fe);
            }
        } catch (...) {
            m_proxy = nullptr;  // mantém a tinta de reserva
        }
    }

    ~BlurBrush()
    {
        if (auto owner = m_owner.get()) {
            try {
                owner.Resources().Remove(winrt::box_value(m_proxyKey));
            } catch (...) {
            }
        }
    }

    // Sobrescritas de XamlCompositionBrushBase.
    void OnConnected()
    {
        if (!CompositionBrush()) {
            RefreshTint();
            CompositionBrush(AdvancedEffectsEnabled() ? CreateEffectBrush() : CreateFallbackBrush());
        }
    }

    void OnDisconnected()
    {
        if (auto brush = CompositionBrush()) {
            brush.Close();
            CompositionBrush(nullptr);
        }
    }

private:
    void RefreshTint()
    {
        if (!m_proxy) {
            return;
        }
        try {
            auto c = m_proxy.Color();
            m_tint = winrt::Windows::UI::Color{m_tintAlpha, c.R, c.G, c.B};
        } catch (...) {
        }
    }

    static bool AdvancedEffectsEnabled()
    {
        try {
            return winrt::Windows::UI::ViewManagement::UISettings().AdvancedEffectsEnabled();
        } catch (...) {
            return true;
        }
    }

    wuc::CompositionBrush CreateFallbackBrush()
    {
        // Sem acesso ao fundo: uma versão quase opaca da tinta.
        winrt::Windows::UI::Color c = m_tint;
        c.A = std::max<uint8_t>(c.A, 0xB0);
        return m_compositor.CreateColorBrush(c);
    }

    wuc::CompositionBrush CreateEffectBrush()
    {
        // 1. Desfoca o fundo. A ordem dos índices das propriedades importa:
        //    D2D1_GAUSSIANBLUR_PROP_STANDARD_DEVIATION=0, OPTIMIZATION=1, BORDER_MODE=2.
        auto blur = winrt::make_self<D2DEffect>(CLSID_D2D1GaussianBlur, L"Blur");
        blur->Property(L"BlurAmount", PropFloat(m_blurAmount))
            .Property(L"Optimization", PropUInt32(1 /* D2D1_GAUSSIANBLUR_OPTIMIZATION_BALANCED; MinGW lacks the enum */))
            .Property(L"BorderMode", PropUInt32(D2D1_BORDER_MODE_SOFT))
            .Source(wuc::CompositionEffectSourceParameter(L"backdrop"));

        // 2. A tinta como uma camada de cor lisa.
        auto flood = winrt::make_self<D2DEffect>(CLSID_D2D1Flood, L"Tint");
        flood->Property(L"Color", PropColor(m_tint.R, m_tint.G, m_tint.B, m_tint.A));

        // 3. Tinta sobre o desfoque.
        auto composite = winrt::make_self<D2DEffect>(CLSID_D2D1Composite, L"Composite");
        composite->Property(L"Mode", PropUInt32(D2D1_COMPOSITE_MODE_SOURCE_OVER))
            .Source(*blur)
            .Source(*flood);

        auto factory = m_compositor.CreateEffectFactory(*composite);
        auto brush = factory.CreateBrush();
        brush.SetSourceParameter(L"backdrop", m_compositor.CreateBackdropBrush());
        return brush;
    }

    wuc::Compositor m_compositor;
    float m_blurAmount;
    winrt::Windows::UI::Color m_tint;
    uint8_t m_tintAlpha;
    winrt::Windows::UI::Color m_fallback;

    wuxm::SolidColorBrush m_proxy{nullptr};
    winrt::weak_ref<wux::FrameworkElement> m_owner;
    winrt::hstring m_proxyKey;
};

}  // namespace tws
