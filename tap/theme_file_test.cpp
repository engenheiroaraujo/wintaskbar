#include <windows.h>
#include "theme_file.h"
#include "dock_geometry.h"
#include <fstream>
#include <iostream>
#include <sstream>
#include <tuple>

static void Check(bool ok, const char* message) {
    if (!ok) throw std::runtime_error(message);
}
static std::wstring Wide(std::string const& bytes) {
    int count = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, bytes.data(), int(bytes.size()), nullptr, 0);
    Check(count > 0, "UTF-8 fixture");
    std::wstring out(count, L'\0');
    MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, bytes.data(), int(bytes.size()), out.data(), count);
    return out;
}
static tws::CompiledTheme Parse(std::wstring const& value) {
    std::wistringstream stream(value); return tws::ReadTheme(stream);
}
static void Reject(std::wstring const& value) {
    try { Parse(value); } catch (std::exception const&) { return; }
    throw std::runtime_error("malformed contract was accepted");
}

static void TestDockGeometry() {
    // Casos de regressão com as larguras informadas na investigação.
    for (auto [width, tray, dock] : {std::tuple{980.0, 741.0, 633.0},
                                     std::tuple{1162.0, 923.0, 853.0},
                                     std::tuple{1154.0, 915.0, 809.0},
                                     std::tuple{1488.0, 1249.0, 853.0}}) {
        double base = (width - dock) / 2;
        auto fit = tws::FitDock(width, tray, 12, dock, base, INFINITY);
        Check(base + fit.offset >= 0, "dock stays inside left edge");
        Check(base + fit.offset + std::min(dock, fit.maxWidth) <= tray - 12 + 0.001,
              "dock stays before tray with gap");
        Check(fit.maxWidth == width, "native tray reservation is not deducted twice");
        if (width == 1488) Check(fit.offset == 0, "wide dock remains centered");
        // Repetir a mesma medição não acumula deslocamentos.
        auto repeat = tws::FitDock(width, tray, 12, dock, base, INFINITY);
        Check(repeat.offset == fit.offset && repeat.maxWidth == fit.maxWidth, "stable layout");
    }
    auto narrow = tws::FitDock(980, 741, 12, 800, 90, 980);
    Check(narrow.maxWidth == 909 && narrow.offset == -90, "only measured excess constrains width");
    auto settled = tws::FitDock(980, 741, 12, 729, 125.5, narrow.maxWidth);
    Check(settled.maxWidth == narrow.maxWidth && settled.offset == -125.5, "constraint settles");
    auto wide = tws::FitDock(1800, 1561, 12, 1200, 300, INFINITY);
    Check(wide.maxWidth == 1800 && wide.offset == 0, "width recovers after resize");
    Check(tws::FitDock(980, 741, 12, 500, 240, 500).maxWidth == 500, "theme MaxWidth preserved");
    Check(tws::FitDock(100, 20, 30, 100, 0, INFINITY).maxWidth == 0, "no negative width");
    auto fractional = tws::FitDock(784, 592.8, 1.5, 506.4, 138.8, INFINITY);
    Check(std::abs(138.8 + fractional.offset + 506.4 - 591.3) < 0.001, "logical DPI and fractional gap");
}
int main(int argc, char** argv) {
    try {
        TestDockGeometry();
        Check(argc == 2, "provide the Go-generated contract fixture");
        std::ifstream file(argv[1], std::ios::binary);
        Check(bool(file), "fixture not found");
        std::string bytes((std::istreambuf_iterator<char>(file)), {});
        auto theme = Parse(Wide(bytes));
        Check(theme.name == L"default" && theme.rules.size() == 12, "default theme rules");
        auto const& dock = theme.rules.front();
        Check(dock.alts[0].size() == 2 && dock.alts[0][1].name == L"RootGrid", "selector hierarchy");
        Check(dock.alts[0][1].typeNames[0] == L"Windows.UI.Xaml.Controls.Grid", "expanded class names");
        auto const& blur = dock.styles[4];
        Check(blur.kind == L"blur" && blur.amount == 28 && blur.alpha == 77 && blur.fallback == 0x4D202020 && blur.themeKey == L"SystemChromeAltHighColor", "blur parameters");
        Check(theme.rules[2].alts.size() == 2, "selector alternatives");
        bool hasLayout = false;
        for (auto const& rule : theme.rules) for (auto const& style : rule.styles)
            if (style.kind == L"dock-layout") {
                Check(style.gap == 12 && style.prop == L"DockLayoutGap", "default adaptive gap");
                hasLayout = true;
            }
        Check(hasLayout, "default theme enables adaptive layout");
        auto fields = tws::Fields(L"text\ta\\tb\\nc\\rd\\\\e");
        Check(fields[1] == L"a\tb\nc\rd\\e", "escaped fields");
        std::wstring prefix = L"wintaskbar-theme\t1\nname\tcustom\nrule\tGrid#Custom\nalt\nc\tWindows.UI.Xaml.Controls.Grid\tCustom\na\tText\tHello\\tworld\n";
        auto custom = Parse(prefix + L"style\tOpacity\tplain\t0.5\nend\n");
        Check(custom.rules[0].styles[0].value == L"0.5" && custom.rules[0].alts[0][0].attrs[0].value == L"Hello\tworld", "custom theme and attributes");
        Reject(L"wintaskbar-theme\t3\n");
        std::wstring layoutPrefix = L"wintaskbar-theme\t2\nname\tlayout\nrule\tTaskbar.TaskbarFrame\nalt\nc\tTaskbar.TaskbarFrame\t\n";
        Check(Parse(layoutPrefix + L"style\tDockLayoutGap\tdock-layout\t1.5\nend\n").rules[0].styles[0].gap == 1.5,
              "fractional layout contract");
        for (auto value : {L"-1", L"1001", L"nan", L"inf", L"12px"})
            Reject(layoutPrefix + L"style\tDockLayoutGap\tdock-layout\t" + value + L"\nend\n");
        Reject(prefix + L"style\tDockLayoutGap\tdock-layout\t12\nend\n");
        auto wrongTarget = prefix;
        wrongTarget.replace(0, std::wstring(L"wintaskbar-theme\t1").size(), L"wintaskbar-theme\t2");
        Reject(wrongTarget + L"style\tDockLayoutGap\tdock-layout\t12\nend\n");
        Reject(layoutPrefix + L"style\tDockLayoutGap\tplain\t12\nend\n");
        Reject(prefix); // truncated file
        Reject(prefix + L"style\tBackground\tblur\tnan\t\t255\tFFFFFFFF\nend\n");
        Reject(prefix + L"style\tBackground\tblur\t20\t\t256\tFFFFFFFF\nend\n");
        Reject(prefix + L"style\tOpacity\tother\t0.5\nend\n");
        Reject(prefix + L"style\tOpacity\tplain\t0.5\nend\nrule\tGrid\n");
        Reject(L"wintaskbar-theme\t1\nname\tx\nrule\tGrid\nalt\nstyle\tOpacity\tplain\t0.5\nend\n");
        std::cout << "Compiled theme contract: all checks passed\n";
        return 0;
    } catch (std::exception const& e) {
        std::cerr << e.what() << '\n'; return 1;
    }
}
