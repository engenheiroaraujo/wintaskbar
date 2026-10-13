// Versões 1 e 2 do contrato Go -> TAP. Sem dependências de XAML/WinRT: testado em
// um processo separado antes de este parser rodar dentro do Explorer.
#pragma once
#include <cmath>
#include <cstdint>
#include <istream>
#include <stdexcept>
#include <string>
#include <vector>

namespace tws {
struct Attr { std::wstring prop, value; };
struct Compound {
    std::wstring name;
    std::vector<Attr> attrs;
    std::vector<std::wstring> typeNames;
};
using Chain = std::vector<Compound>;
struct Style {
    std::wstring prop, value, kind, themeKey;
    float amount = 0;
    uint8_t alpha = 255;
    uint32_t fallback = 0;
    double gap = 0;
};
struct Rule {
    std::wstring selector;
    std::vector<Chain> alts;
    std::vector<Style> styles;
};
struct CompiledTheme { std::wstring name; std::vector<Rule> rules; };

inline std::vector<std::wstring> Fields(std::wstring const& line) {
    std::vector<std::wstring> out(1);
    for (size_t i = 0; i < line.size(); ++i) {
        wchar_t c = line[i];
        if (c == L'\t') { out.emplace_back(); continue; }
        if (c == L'\\') {
            if (++i == line.size()) throw std::runtime_error("truncated escape");
            switch (line[i]) {
            case L'\\': c = L'\\'; break;
            case L't': c = L'\t'; break;
            case L'r': c = L'\r'; break;
            case L'n': c = L'\n'; break;
            default: throw std::runtime_error("invalid escape");
            }
        }
        out.back() += c;
    }
    return out;
}

inline CompiledTheme ReadTheme(std::wistream& input) {
    auto require = [](bool ok) { if (!ok) throw std::runtime_error("invalid compiled theme"); };
    std::wstring line;
    require(bool(std::getline(input, line)));
    bool version2 = line == L"wintaskbar-theme\t2";
    require(version2 || line == L"wintaskbar-theme\t1");
    require(bool(std::getline(input, line)));
    auto name = Fields(line);
    require(name.size() == 2 && name[0] == L"name" && !name[1].empty());
    CompiledTheme theme{name[1], {}};
    bool ended = false;
    size_t records = 0;
    while (std::getline(input, line)) {
        require(!ended && ++records < 100000 && line.size() < 1024 * 1024);
        auto f = Fields(line);
        if (f[0] == L"end") { require(f.size() == 1); ended = true; continue; }
        if (f[0] == L"rule") {
            require(f.size() == 2 && !f[1].empty());
            theme.rules.push_back({f[1], {}, {}});
            continue;
        }
        require(!theme.rules.empty());
        auto& r = theme.rules.back();
        if (f[0] == L"alt") {
            require(f.size() == 1 && r.styles.empty()); r.alts.emplace_back();
        } else if (f[0] == L"c") {
            require(f.size() == 3 && !r.alts.empty() && r.styles.empty());
            Compound c; c.name = f[2];
            size_t start = 0;
            do {
                auto end = f[1].find(L'|', start);
                auto type = f[1].substr(start, end == std::wstring::npos ? end : end - start);
                require(!type.empty()); c.typeNames.push_back(type);
                if (end == std::wstring::npos) break;
                start = end + 1;
            } while (true);
            r.alts.back().push_back(std::move(c));
        } else if (f[0] == L"a") {
            require(f.size() == 3 && !r.alts.empty() && !r.alts.back().empty() && r.styles.empty());
            require(f[1] == L"Text" || f[1] == L"AutomationProperties.AutomationId");
            r.alts.back().back().attrs.push_back({f[1], f[2]});
        } else if (f[0] == L"style") {
            require(f.size() >= 4 && !f[1].empty());
            Style s; s.prop = f[1]; s.kind = f[2];
            require(s.prop != L"DockLayoutGap" || s.kind == L"dock-layout");
            if (s.kind == L"plain" || s.kind == L"xaml") {
                require(f.size() == 4 && !f[3].empty()); s.value = f[3];
            } else if (s.kind == L"dock-layout") {
                require(version2 && f.size() == 4 && s.prop == L"DockLayoutGap");
                size_t n = 0;
                s.gap = std::stod(f[3], &n);
                require(n == f[3].size() && std::isfinite(s.gap) && s.gap >= 0 && s.gap <= 1000);
                for (auto const& chain : r.alts) {
                    require(!chain.empty() && chain.back().typeNames.size() == 1 &&
                            chain.back().typeNames[0] == L"Taskbar.TaskbarFrame");
                }
            } else {
                require(s.kind == L"blur" && f.size() == 7);
                size_t n = 0;
                s.amount = std::stof(f[3], &n);
                require(n == f[3].size() && std::isfinite(s.amount) && s.amount >= 0 && s.amount <= 250);
                s.themeKey = f[4];
                auto alpha = std::stoul(f[5], &n); require(n == f[5].size() && alpha <= 255);
                s.alpha = static_cast<uint8_t>(alpha);
                auto color = std::stoull(f[6], &n, 16); require(n == 8 && f[6].size() == 8 && color <= UINT32_MAX);
                s.fallback = static_cast<uint32_t>(color);
            }
            r.styles.push_back(std::move(s));
        } else { require(false); }
    }
    require(ended && !theme.rules.empty());
    for (auto const& r : theme.rules) {
        require(!r.alts.empty() && !r.styles.empty());
        for (auto const& c : r.alts) require(!c.empty());
    }
    return theme;
}
} // namespace tws
