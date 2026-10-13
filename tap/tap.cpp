// wintaskbar TAP (Tool Access Point).
//
// Esta DLL é carregada DENTRO do explorer.exe pelo runtime de diagnóstico XAML do Windows
// (disparada pelo agent em Go via InitializeXamlDiagnosticsEx). Ela recebe um
// site IXamlDiagnostics, assina as mudanças da árvore visual e entrega cada novo
// elemento ao Styler, que aplica as regras do tema compilado (theme.compiled).
//
// A técnica usa a API de diagnóstico XAML do Windows: um objeto COM TAP
// (IObjectWithSite + IVisualTreeServiceCallback2) recebe cada elemento da árvore
// visual. O código foi escrito do zero para este projeto.

#include <windows.h>
#include <unknwn.h>
#include <ocidl.h>
#include <xamlom.h>

// winbase.h define GetCurrentTime() como macro, o que quebra os headers de
// Animation do C++/WinRT (eles declaram um método com esse nome).
#undef GetCurrentTime

#include <cstdarg>
#include <cstdio>
#include <string>
#include <fstream>
#include <sstream>
#include <filesystem>

#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.UI.Xaml.h>
#include <winrt/Windows.UI.Xaml.Controls.h>
#include <winrt/Windows.UI.Xaml.Markup.h>
#include <winrt/Windows.UI.Xaml.Media.h>

#include "styler.h"

namespace wux = winrt::Windows::UI::Xaml;

// {7B1A2C54-3F0E-4D6A-9C1B-5A2E8F4D7B10}
static constexpr CLSID CLSID_WinTaskbarTAP = {
    0x7b1a2c54, 0x3f0e, 0x4d6a, {0x9c, 0x1b, 0x5a, 0x2e, 0x8f, 0x4d, 0x7b, 0x10}};

////////////////////////////////////////////////////////////////////////////////
// Logging: %LOCALAPPDATA%\wintaskbar\tap.log (+ OutputDebugString).

static void Log(const wchar_t* fmt, ...)
{
    wchar_t msg[1024];
    va_list ap;
    va_start(ap, fmt);
    vswprintf(msg, ARRAYSIZE(msg), fmt, ap);
    va_end(ap);

    OutputDebugStringW(msg);
    OutputDebugStringW(L"\n");

    wchar_t dir[MAX_PATH];
    DWORD n = GetEnvironmentVariableW(L"LOCALAPPDATA", dir, ARRAYSIZE(dir));
    if (n == 0 || n >= ARRAYSIZE(dir)) {
        return;
    }
    std::wstring folder = std::wstring(dir) + L"\\wintaskbar";
    CreateDirectoryW(folder.c_str(), nullptr);
    std::wstring file = folder + L"\\tap.log";

    HANDLE h = CreateFileW(file.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE,
                           nullptr, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (h == INVALID_HANDLE_VALUE) {
        return;
    }
    SYSTEMTIME t;
    GetLocalTime(&t);
    wchar_t line[1200];
    int len = swprintf(line, ARRAYSIZE(line), L"%02d:%02d:%02d.%03d [%lu] %ls\r\n", t.wHour, t.wMinute,
                       t.wSecond, t.wMilliseconds, GetCurrentThreadId(), msg);
    if (len > 0) {
        char utf8[4000];
        int bytes = WideCharToMultiByte(CP_UTF8, 0, line, len, utf8, sizeof(utf8), nullptr, nullptr);
        DWORD written;
        if (bytes > 0) {
            WriteFile(h, utf8, bytes, &written, nullptr);
        }
    }
    CloseHandle(h);
}

////////////////////////////////////////////////////////////////////////////////
// Lê apenas o contrato compilado e validado produzido pelo agent.
static tws::CompiledTheme LoadTheme()
{
    wchar_t local[MAX_PATH];
    DWORD n = GetEnvironmentVariableW(L"LOCALAPPDATA", local, ARRAYSIZE(local));
    if (!n || n >= ARRAYSIZE(local)) throw std::runtime_error("LOCALAPPDATA unavailable");
    std::ifstream file(std::filesystem::path(local) / L"wintaskbar" / L"theme.compiled", std::ios::binary);
    if (!file) throw std::runtime_error("compiled theme not found");
    file.seekg(0, std::ios::end);
    auto size = file.tellg();
    if (size <= 0 || size > 8 * 1024 * 1024) throw std::runtime_error("invalid theme size");
    file.seekg(0);
    std::string bytes(static_cast<size_t>(size), '\0');
    if (!file.read(bytes.data(), bytes.size())) throw std::runtime_error("theme read failed");
    int count = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, bytes.data(), int(bytes.size()), nullptr, 0);
    if (!count) throw std::runtime_error("invalid UTF-8 theme");
    std::wstring wide(count, L'\0');
    MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, bytes.data(), int(bytes.size()), wide.data(), count);
    std::wistringstream input(wide);
    return tws::ReadTheme(input);
}

static tws::Styler& GetStyler()
{
    static tws::Styler styler(LoadTheme(), [](std::wstring const& m) { Log(L"%ls", m.c_str()); });
    return styler;
}

////////////////////////////////////////////////////////////////////////////////
// Observador da árvore visual.

class VisualTreeWatcher : public winrt::implements<VisualTreeWatcher, IVisualTreeServiceCallback2, winrt::non_agile>
{
public:
    explicit VisualTreeWatcher(winrt::com_ptr<IUnknown> const& site) : m_diag(site.as<IXamlDiagnostics>())
    {
        Log(L"VisualTreeWatcher created");
    }

    void Start()
    {
        // AdviseVisualTreeChange pode travar quando chamado na thread que
        // recebeu SetSite, então roda em uma thread dedicada.
        AddRef();
        HANDLE t = CreateThread(
            nullptr, 0,
            [](LPVOID p) -> DWORD {
                auto self = static_cast<VisualTreeWatcher*>(p);
                HRESULT hr = S_OK;
                try {
                    hr = self->m_diag.as<IVisualTreeService3>()->AdviseVisualTreeChange(self);
                } catch (...) {
                    hr = winrt::to_hresult();
                }
                Log(L"AdviseVisualTreeChange -> 0x%08X", static_cast<unsigned>(hr));
                self->Release();
                return 0;
            },
            this, 0, nullptr);
        if (t) {
            CloseHandle(t);
        } else {
            Release();
        }
    }

    void Stop()
    {
        try {
            m_diag.as<IVisualTreeService3>()->UnadviseVisualTreeChange(this);
        } catch (...) {
        }
        GetStyler().StopLayouts();
    }

    HRESULT STDMETHODCALLTYPE OnVisualTreeChange(ParentChildRelation relation, VisualElement element,
                                                 VisualMutationType type) noexcept override
    {
        try {
            auto& styler = GetStyler();
            if (type == Remove) {
                styler.Remove(element.Handle);
                return S_OK;
            }
            if (type != Add || !element.Type) return S_OK;
            std::wstring typeName = element.Type;
            if (!styler.IsCandidate(typeName)) {
                return S_OK;  // pré-filtro barato antes de tocar no objeto
            }

            auto obj = FromHandle(element.Handle);
            winrt::Windows::Foundation::IInspectable parent{nullptr};
            try {
                parent = FromHandle(relation.Parent);
            } catch (...) {
            }
            styler.Apply(obj, typeName, parent, element.Handle);
        } catch (...) {
            Log(L"OnVisualTreeChange error 0x%08X", static_cast<unsigned>(winrt::to_hresult().value));
        }
        return S_OK;  // devolver erro pode interromper as próximas notificações
    }

    HRESULT STDMETHODCALLTYPE OnElementStateChanged(InstanceHandle, VisualElementState, LPCWSTR) noexcept override
    {
        return S_OK;
    }

private:
    winrt::Windows::Foundation::IInspectable FromHandle(InstanceHandle handle)
    {
        winrt::Windows::Foundation::IInspectable obj;
        winrt::check_hresult(m_diag->GetIInspectableFromHandle(
            handle, reinterpret_cast<::IInspectable**>(winrt::put_abi(obj))));
        return obj;
    }

    winrt::com_ptr<IXamlDiagnostics> m_diag;
};

////////////////////////////////////////////////////////////////////////////////
// Objeto COM do TAP + class factory + exports da DLL.

static winrt::com_ptr<VisualTreeWatcher> g_watcher;

static HMODULE ThisModule()
{
    HMODULE m = nullptr;
    GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS | GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT,
                       reinterpret_cast<LPCWSTR>(&ThisModule), &m);
    return m;
}

class StylerTAP : public winrt::implements<StylerTAP, IObjectWithSite, winrt::non_agile>
{
public:
    HRESULT STDMETHODCALLTYPE SetSite(IUnknown* pUnkSite) noexcept override
    {
        try {
            if (g_watcher) {
                g_watcher->Stop();
                g_watcher = nullptr;
            }
            m_site.copy_from(pUnkSite);
            if (m_site) {
                Log(L"SetSite: attached to process %lu", GetCurrentProcessId());
                // Fixa a DLL: threads e vtables COM deste módulo sobrevivem ao
                // SetSite, então ela nunca pode ser descarregada. (NÃO chame FreeLibrary
                // aqui: nada mais mantém uma referência.)
                HMODULE pinned = nullptr;
                GetModuleHandleExW(GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS | GET_MODULE_HANDLE_EX_FLAG_PIN,
                                   reinterpret_cast<LPCWSTR>(&ThisModule), &pinned);
                GetStyler();  // carrega o tema antes da primeira notificação
                g_watcher = winrt::make_self<VisualTreeWatcher>(m_site);
                g_watcher->Start();
            }
            return S_OK;
        } catch (...) {
            HRESULT hr = winrt::to_hresult();
            Log(L"SetSite error 0x%08X", static_cast<unsigned>(hr));
            return hr;
        }
    }

    HRESULT STDMETHODCALLTYPE GetSite(REFIID riid, void** ppvSite) noexcept override
    {
        return m_site.as(riid, ppvSite);
    }

private:
    winrt::com_ptr<IUnknown> m_site;
};

template <typename T>
struct SimpleFactory : winrt::implements<SimpleFactory<T>, IClassFactory, winrt::non_agile>
{
    HRESULT STDMETHODCALLTYPE CreateInstance(IUnknown* outer, REFIID riid, void** ppv) noexcept override
    {
        if (outer) {
            return CLASS_E_NOAGGREGATION;
        }
        try {
            *ppv = nullptr;
            return winrt::make<T>().as(riid, ppv);
        } catch (...) {
            return winrt::to_hresult();
        }
    }
    HRESULT STDMETHODCALLTYPE LockServer(BOOL) noexcept override { return S_OK; }
};

// combaseapi.h já declara estas duas; exportá-las pela redeclaração
// é intencional.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdll-attribute-on-redeclaration"

extern "C" __declspec(dllexport) HRESULT STDAPICALLTYPE DllGetClassObject(REFCLSID rclsid, REFIID riid, LPVOID* ppv)
{
    if (rclsid == CLSID_WinTaskbarTAP) {
        try {
            *ppv = nullptr;
            return winrt::make<SimpleFactory<StylerTAP>>().as(riid, ppv);
        } catch (...) {
            return winrt::to_hresult();
        }
    }
    return CLASS_E_CLASSNOTAVAILABLE;
}

extern "C" __declspec(dllexport) HRESULT STDAPICALLTYPE DllCanUnloadNow()
{
    return winrt::get_module_lock() ? S_FALSE : S_OK;
}

#pragma clang diagnostic pop
