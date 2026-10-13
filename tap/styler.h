// Aplica o tema compilado aos elementos XAML que casam. A interpretação de seletores e a
// expansão de variáveis pertencem ao agent em Go.
#pragma once

#include "blur_brush.h"
#include "theme_file.h"
#include "dock_layout.h"

#include <cwchar>
#include <cwctype>
#include <functional>
#include <limits>
#include <mutex>
#include <string>
#include <string_view>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include <winrt/Windows.UI.Xaml.Automation.h>
#include <winrt/Windows.UI.Xaml.Controls.h>
#include <winrt/Windows.UI.Xaml.Media.h>
#include <winrt/Windows.UI.Xaml.Shapes.h>

namespace tws
{
namespace wuxc = winrt::Windows::UI::Xaml::Controls;
namespace wuxs = winrt::Windows::UI::Xaml::Shapes;
namespace wuxa = winrt::Windows::UI::Xaml::Automation;
using winrt::Windows::Foundation::IInspectable;

namespace detail
{

inline std::wstring Trim(std::wstring const& s)
{
    auto b = s.find_first_not_of(L" \t\r\n");
    if (b == std::wstring::npos) {
        return L"";
    }
    auto e = s.find_last_not_of(L" \t\r\n");
    return s.substr(b, e - b + 1);
}

// Chama f(x) para o primeiro tipo de Ts como o qual `o` pode ser visto.
template <typename... Ts, typename F>
bool ForAny(IInspectable const& o, F&& f)
{
    bool done = false;
    auto one = [&](auto tag) {
        using T = typename decltype(tag)::type;
        if (!done) {
            if (auto x = o.try_as<T>()) {
                f(x);
                done = true;
            }
        }
    };
    (one(std::type_identity<Ts>{}), ...);
    return done;
}

inline std::vector<double> ParseNumbers(std::wstring const& v)
{
    std::vector<double> out;
    std::wstring cur;
    auto flush = [&]() {
        if (!cur.empty()) {
            out.push_back(wcstod(cur.c_str(), nullptr));
            cur.clear();
        }
    };
    for (wchar_t c : v) {
        if (c == L',' || c == L' ') flush();
        else cur += c;
    }
    flush();
    return out;
}

inline bool ParseThickness(std::wstring const& v, winrt::Windows::UI::Xaml::Thickness& t)
{
    auto n = ParseNumbers(v);
    if (n.size() == 1) t = {n[0], n[0], n[0], n[0]};
    else if (n.size() == 2) t = {n[0], n[1], n[0], n[1]};
    else if (n.size() == 4) t = {n[0], n[1], n[2], n[3]};
    else return false;
    return true;
}

inline bool ParseCornerRadius(std::wstring const& v, winrt::Windows::UI::Xaml::CornerRadius& r)
{
    auto n = ParseNumbers(v);
    if (n.size() == 1) r = {n[0], n[0], n[0], n[0]};
    else if (n.size() == 4) r = {n[0], n[1], n[2], n[3]};
    else return false;
    return true;
}

inline bool ParseDouble(std::wstring const& v, double& d)
{
    if (_wcsicmp(v.c_str(), L"Auto") == 0) {
        d = std::numeric_limits<double>::quiet_NaN();
        return true;
    }
    wchar_t* end = nullptr;
    d = wcstod(v.c_str(), &end);
    return end && end != v.c_str() && *end == 0;
}

}  // namespace detail

class Styler
{
public:
    using LogFn = std::function<void(std::wstring const&)>;

    Styler(CompiledTheme theme, LogFn log) : m_log(std::move(log)), m_rules(std::move(theme.rules))
    {
        for (size_t i = 0; i < m_rules.size(); ++i) {
            for (size_t a = 0; a < m_rules[i].alts.size(); ++a) {
                for (auto const& type : m_rules[i].alts[a].back().typeNames)
                    m_index[type].push_back({i, a});
            }
        }
        m_log(L"theme: " + theme.name + L", " + std::to_wstring(m_rules.size()) + L" rules loaded");
    }

    bool IsCandidate(std::wstring const& typeName) const { return m_index.find(typeName) != m_index.end(); }

    // `typeName` é o nome de classe do elemento em tempo de execução, `parent` seu pai visual
    // (pode ser nulo).
    void Apply(IInspectable const& element, std::wstring const& typeName, IInspectable const& parent,
               uint64_t handle)
    {
        auto it = m_index.find(typeName);
        if (it == m_index.end()) {
            return;
        }

        std::vector<IInspectable> anc{parent};
        std::vector<std::wstring> ancNames{L""};
        auto ancestor = [&](size_t k) -> IInspectable {
            while (anc.size() <= k) {
                IInspectable next{nullptr};
                if (auto last = anc.back()) {
                    if (auto dep = last.try_as<winrt::Windows::UI::Xaml::DependencyObject>()) {
                        try {
                            next = winrt::Windows::UI::Xaml::Media::VisualTreeHelper::GetParent(dep);
                        } catch (...) {
                        }
                    }
                }
                anc.push_back(next);
                ancNames.push_back(L"");
            }
            if (anc[k] && ancNames[k].empty()) {
                ancNames[k] = std::wstring{winrt::get_class_name(anc[k])};
            }
            return anc[k];
        };

        size_t lastRule = SIZE_MAX;
        double layoutGap = -1;
        for (auto const& [ruleIdx, altIdx] : it->second) {
            if (ruleIdx == lastRule) {
                continue;  // uma alternativa anterior desta regra já foi aplicada
            }
            auto const& rule = m_rules[ruleIdx];
            auto const& chain = rule.alts[altIdx];
            if (!MatchChain(chain, element, typeName, ancestor, ancNames)) {
                continue;
            }
            lastRule = ruleIdx;
            if (m_matched.insert(rule.selector).second) {
                m_log(L"match: " + rule.selector);  // primeira vez que esta regra casa com um elemento
            }
            for (auto const& st : rule.styles) {
                if (st.kind == L"dock-layout") layoutGap = st.gap;
                else ApplyStyle(element, rule, st);
            }
        }
        // Captura os valores originais somente depois de todos os Styles fixos.
        if (layoutGap >= 0) {
            Remove(handle);
            auto layout = DockLayout::Attach(element.as<winrt::Windows::UI::Xaml::FrameworkElement>(),
                                             layoutGap, m_log);
            std::lock_guard lock(m_layoutMutex);
            m_layouts[handle] = layout;
        }
    }

    void Remove(uint64_t handle) {
        std::shared_ptr<DockLayout> layout;
        {
            std::lock_guard lock(m_layoutMutex);
            auto it = m_layouts.find(handle);
            if (it == m_layouts.end()) return;
            layout = it->second.lock();
            m_layouts.erase(it);
        }
        if (layout) layout->Stop();
    }

    void StopLayouts() {
        std::vector<std::shared_ptr<DockLayout>> layouts;
        {
            std::lock_guard lock(m_layoutMutex);
            for (auto const& [handle, weak] : m_layouts)
                if (auto layout = weak.lock()) layouts.push_back(std::move(layout));
            m_layouts.clear();
        }
        for (auto const& layout : layouts) layout->Stop();
    }

private:
    template <typename AncFn>
    bool MatchChain(Chain const& chain, IInspectable const& element, std::wstring const& typeName,
                    AncFn& ancestor, std::vector<std::wstring> const& ancNames)
    {
        size_t n = chain.size();
        if (!MatchCompound(chain[n - 1], element, typeName)) {
            return false;
        }
        for (size_t k = 0; k + 1 < n; k++) {
            auto a = ancestor(k);
            if (!a) {
                return false;
            }
            if (!MatchCompound(chain[n - 2 - k], a, ancNames[k])) {
                return false;
            }
        }
        return true;
    }

    static bool MatchCompound(Compound const& c, IInspectable const& obj, std::wstring const& className)
    {
        bool typeOk = false;
        for (auto const& t : c.typeNames) {
            if (t == className) {
                typeOk = true;
                break;
            }
        }
        if (!typeOk) {
            return false;
        }
        if (!c.name.empty()) {
            auto fe = obj.try_as<winrt::Windows::UI::Xaml::FrameworkElement>();
            if (!fe || std::wstring{fe.Name()} != c.name) {
                return false;
            }
        }
        for (auto const& a : c.attrs) {
            if (a.prop == L"Text") {
                auto tb = obj.try_as<wuxc::TextBlock>();
                if (!tb || std::wstring{tb.Text()} != a.value) return false;
            } else if (a.prop == L"AutomationProperties.AutomationId") {
                auto dep = obj.try_as<winrt::Windows::UI::Xaml::DependencyObject>();
                if (!dep || std::wstring{wuxa::AutomationProperties::GetAutomationId(dep)} != a.value) {
                    return false;
                }
            } else {
                return false;  // atributo não suportado: nunca casa
            }
        }
        return true;
    }

    void ReportOnce(Rule const& rule, Style const& st, std::wstring const& why)
    {
        std::wstring key = rule.selector + L"|" + st.prop;
        if (m_reported.insert(key).second) {
            m_log(L"skip: " + rule.selector + L" :: " + st.prop + L" (" + why + L")");
        }
    }

    void ApplyStyle(IInspectable const& obj, Rule const& rule, Style const& st)
    {
        try {
            if (st.kind == L"blur") {
                auto color = st.fallback;
                winrt::Windows::UI::Color fallback{uint8_t(color >> 24), uint8_t(color >> 16),
                                                    uint8_t(color >> 8), uint8_t(color)};
                auto brush = winrt::make<BlurBrush>(obj.as<winrt::Windows::UI::Xaml::UIElement>(),
                                                    st.amount, st.themeKey, st.alpha, fallback);
                if (!SetBrushProperty(obj, st.prop, brush)) ReportOnce(rule, st, L"unsupported brush target");
            } else if (!SetProperty(obj, st.prop, st.value, st.kind == L"xaml")) {
                ReportOnce(rule, st, L"unsupported property/target");
            }
        } catch (...) {
            ReportOnce(rule, st, L"error 0x" + std::to_wstring(static_cast<unsigned>(winrt::to_hresult().value)));
        }
    }

    // -- definição de propriedades ---------------------------------------------------

    static winrt::Windows::UI::Xaml::Media::Brush BrushFromPlain(std::wstring const& v)
    {
        std::wstring xaml = L"<SolidColorBrush xmlns='http://schemas.microsoft.com/winfx/2006/xaml/presentation' "
                            L"Color='" +
                            v + L"'/>";
        return winrt::Windows::UI::Xaml::Markup::XamlReader::Load(xaml)
            .as<winrt::Windows::UI::Xaml::Media::Brush>();
    }

    bool SetBrushProperty(IInspectable const& o, std::wstring const& prop,
                          winrt::Windows::UI::Xaml::Media::Brush const& b)
    {
        using detail::ForAny;
        if (prop == L"Background") {
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Panel, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.Background(b); });
        }
        if (prop == L"BorderBrush") {
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Grid, wuxc::StackPanel, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.BorderBrush(b); });
        }
        if (prop == L"Foreground") {
            return ForAny<wuxc::Control, wuxc::TextBlock, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.Foreground(b); });
        }
        if (prop == L"Fill") {
            return ForAny<wuxs::Shape>(o, [&](auto const& x) { x.Fill(b); });
        }
        if (prop == L"Stroke") {
            return ForAny<wuxs::Shape>(o, [&](auto const& x) { x.Stroke(b); });
        }
        return false;
    }

    static bool IsBrushProperty(std::wstring const& p)
    {
        return p == L"Background" || p == L"BorderBrush" || p == L"Foreground" || p == L"Fill" || p == L"Stroke";
    }

    bool SetProperty(IInspectable const& o, std::wstring const& prop, std::wstring const& v, bool xaml)
    {
        using detail::ForAny;
        namespace wux = winrt::Windows::UI::Xaml;

        // XAML objects.
        if (xaml) {
            auto parsed = wux::Markup::XamlReader::Load(v);
            if (IsBrushProperty(prop)) {
                return SetBrushProperty(o, prop, parsed.as<wux::Media::Brush>());
            }
            if (prop == L"RenderTransform") {
                if (auto ui = o.try_as<wux::UIElement>()) {
                    ui.RenderTransform(parsed.as<wux::Media::Transform>());
                    return true;
                }
            }
            return false;
        }

        // Plain values.
        if (IsBrushProperty(prop)) {
            return SetBrushProperty(o, prop, BrushFromPlain(v));
        }

        if (prop == L"Margin") {
            wux::Thickness t;
            auto fe = o.try_as<wux::FrameworkElement>();
            if (!fe || !detail::ParseThickness(v, t)) return false;
            fe.Margin(t);
            return true;
        }
        if (prop == L"Padding") {
            wux::Thickness t;
            if (!detail::ParseThickness(v, t)) return false;
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Grid, wuxc::StackPanel, wuxc::ContentPresenter,
                          wuxc::TextBlock>(o, [&](auto const& x) { x.Padding(t); });
        }
        if (prop == L"BorderThickness") {
            wux::Thickness t;
            if (!detail::ParseThickness(v, t)) return false;
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Grid, wuxc::StackPanel, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.BorderThickness(t); });
        }
        if (prop == L"CornerRadius") {
            wux::CornerRadius r;
            if (!detail::ParseCornerRadius(v, r)) return false;
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Grid, wuxc::StackPanel, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.CornerRadius(r); });
        }
        if (prop == L"BackgroundSizing") {
            auto s = _wcsicmp(v.c_str(), L"OuterBorderEdge") == 0 ? wuxc::BackgroundSizing::OuterBorderEdge
                                                                   : wuxc::BackgroundSizing::InnerBorderEdge;
            return ForAny<wuxc::Control, wuxc::Border, wuxc::Grid, wuxc::StackPanel, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.BackgroundSizing(s); });
        }

        if (prop == L"Width" || prop == L"Height" || prop == L"MinWidth" || prop == L"MinHeight" ||
            prop == L"MaxWidth" || prop == L"MaxHeight") {
            double d;
            auto fe = o.try_as<wux::FrameworkElement>();
            if (!fe || !detail::ParseDouble(v, d)) return false;
            if (prop == L"Width") fe.Width(d);
            else if (prop == L"Height") fe.Height(d);
            else if (prop == L"MinWidth") fe.MinWidth(d);
            else if (prop == L"MinHeight") fe.MinHeight(d);
            else if (prop == L"MaxWidth") fe.MaxWidth(d);
            else fe.MaxHeight(d);
            return true;
        }
        if (prop == L"Opacity") {
            double d;
            auto ui = o.try_as<wux::UIElement>();
            if (!ui || !detail::ParseDouble(v, d)) return false;
            ui.Opacity(d);
            return true;
        }
        if (prop == L"Visibility") {
            auto ui = o.try_as<wux::UIElement>();
            if (!ui) return false;
            bool collapsed = v == L"1" || _wcsicmp(v.c_str(), L"Collapsed") == 0;
            ui.Visibility(collapsed ? wux::Visibility::Collapsed : wux::Visibility::Visible);
            return true;
        }
        if (prop == L"HorizontalAlignment" || prop == L"VerticalAlignment") {
            auto fe = o.try_as<wux::FrameworkElement>();
            if (!fe) return false;
            int n = -1;
            if (_wcsicmp(v.c_str(), L"Left") == 0 || _wcsicmp(v.c_str(), L"Top") == 0) n = 0;
            else if (_wcsicmp(v.c_str(), L"Center") == 0) n = 1;
            else if (_wcsicmp(v.c_str(), L"Right") == 0 || _wcsicmp(v.c_str(), L"Bottom") == 0) n = 2;
            else if (_wcsicmp(v.c_str(), L"Stretch") == 0) n = 3;
            else if (v.size() == 1 && v[0] >= L'0' && v[0] <= L'3') n = v[0] - L'0';
            if (n < 0) return false;
            if (prop == L"HorizontalAlignment") fe.HorizontalAlignment(static_cast<wux::HorizontalAlignment>(n));
            else fe.VerticalAlignment(static_cast<wux::VerticalAlignment>(n));
            return true;
        }
        if (prop == L"FontSize") {
            double d;
            if (!detail::ParseDouble(v, d)) return false;
            return ForAny<wuxc::Control, wuxc::TextBlock, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.FontSize(d); });
        }
        if (prop == L"FontFamily") {
            wux::Media::FontFamily f{v};
            return ForAny<wuxc::Control, wuxc::TextBlock, wuxc::ContentPresenter>(
                o, [&](auto const& x) { x.FontFamily(f); });
        }
        if (prop == L"Text") {
            return ForAny<wuxc::TextBlock>(o, [&](auto const& x) { x.Text(v); });
        }
        return false;
    }

    LogFn m_log;
    std::vector<Rule> m_rules;
    std::unordered_map<std::wstring, std::vector<std::pair<size_t, size_t>>> m_index;
    std::unordered_set<std::wstring> m_reported;
    std::unordered_set<std::wstring> m_matched;
    std::mutex m_layoutMutex;
    std::unordered_map<uint64_t, std::weak_ptr<DockLayout>> m_layouts;
};

}  // namespace tws
