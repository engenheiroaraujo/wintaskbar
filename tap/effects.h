// Suporte mínimo, sem Win2D, a grafos de efeitos do Windows.UI.Composition.
//
// CompositionEffectFactory aceita qualquer objeto que implemente IGraphicsEffect
// mais a interface de interop D2D1 abaixo. Uma classe orientada a dados (D2DEffect)
// descreve qualquer efeito D2D: seu CLSID, suas propriedades nomeadas e suas fontes.

#pragma once

#include <windows.h>
#include <initguid.h>  // define os CLSIDs dos efeitos D2D nesta unidade de tradução
#include <d2d1_1.h>
#include <d2d1effects.h>
#include <windows.graphics.effects.h>

#include <string>
#include <vector>

#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Graphics.Effects.h>

namespace tws
{
namespace wge = winrt::Windows::Graphics::Effects;
namespace awge = ABI::Windows::Graphics::Effects;

enum EffectPropertyMapping : int32_t
{
    kMappingUnknown,
    kMappingDirect,
};

// IGraphicsEffectD2D1Interop (windows.graphics.effects.interop.h), que os
// headers do MinGW não trazem.
struct IGraphicsEffectD2D1Interop : IUnknown
{
    virtual HRESULT STDMETHODCALLTYPE GetEffectId(GUID* id) noexcept = 0;
    virtual HRESULT STDMETHODCALLTYPE GetNamedPropertyMapping(LPCWSTR name, UINT* index,
                                                              EffectPropertyMapping* mapping) noexcept = 0;
    virtual HRESULT STDMETHODCALLTYPE GetPropertyCount(UINT* count) noexcept = 0;
    virtual HRESULT STDMETHODCALLTYPE GetProperty(
        UINT index, winrt::impl::abi_t<winrt::Windows::Foundation::IPropertyValue>** value) noexcept = 0;
    virtual HRESULT STDMETHODCALLTYPE GetSource(UINT index, awge::IGraphicsEffectSource** source) noexcept = 0;
    virtual HRESULT STDMETHODCALLTYPE GetSourceCount(UINT* count) noexcept = 0;
};

}  // namespace tws

template <>
inline constexpr winrt::guid winrt::impl::guid_v<tws::IGraphicsEffectD2D1Interop>{
    0x2FC57384, 0xA068, 0x44D7, {0xA3, 0x31, 0x30, 0x98, 0x2F, 0xCF, 0x71, 0x77}};

namespace tws
{

// Descreve um efeito D2D. As propriedades são endereçadas pelo índice D2D,
// que também é sua ordem: adicione-as em ordem de índice a partir de 0.
struct D2DEffect : winrt::implements<D2DEffect, wge::IGraphicsEffect, wge::IGraphicsEffectSource,
                                     IGraphicsEffectD2D1Interop>
{
    D2DEffect(GUID const& id, winrt::hstring name) : m_id(id), m_name(std::move(name)) {}

    // Acrescenta a próxima propriedade (seu índice é o número de propriedades até agora).
    D2DEffect& Property(wchar_t const* name, winrt::Windows::Foundation::IPropertyValue value)
    {
        m_props.push_back({name, std::move(value)});
        return *this;
    }

    D2DEffect& Source(wge::IGraphicsEffectSource const& source)
    {
        m_sources.push_back(source);
        return *this;
    }

    // IGraphicsEffect
    winrt::hstring Name() { return m_name; }
    void Name(winrt::hstring const& name) { m_name = name; }

    // IGraphicsEffectD2D1Interop
    HRESULT STDMETHODCALLTYPE GetEffectId(GUID* id) noexcept override
    {
        if (!id) return E_INVALIDARG;
        *id = m_id;
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE GetNamedPropertyMapping(LPCWSTR name, UINT* index,
                                                      EffectPropertyMapping* mapping) noexcept override
    {
        if (!name || !index || !mapping) return E_INVALIDARG;
        for (size_t i = 0; i < m_props.size(); i++) {
            if (m_props[i].name == name) {
                *index = static_cast<UINT>(i);
                *mapping = kMappingDirect;
                return S_OK;
            }
        }
        return E_INVALIDARG;
    }

    HRESULT STDMETHODCALLTYPE GetPropertyCount(UINT* count) noexcept override
    {
        if (!count) return E_INVALIDARG;
        *count = static_cast<UINT>(m_props.size());
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE GetProperty(
        UINT index, winrt::impl::abi_t<winrt::Windows::Foundation::IPropertyValue>** value) noexcept override
    {
        if (!value) return E_INVALIDARG;
        if (index >= m_props.size()) return E_BOUNDS;
        winrt::copy_to_abi(m_props[index].value, *reinterpret_cast<void**>(value));
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE GetSource(UINT index, awge::IGraphicsEffectSource** source) noexcept override
    {
        if (!source) return E_INVALIDARG;
        if (index >= m_sources.size()) return E_BOUNDS;
        winrt::copy_to_abi(m_sources[index], *reinterpret_cast<void**>(source));
        return S_OK;
    }

    HRESULT STDMETHODCALLTYPE GetSourceCount(UINT* count) noexcept override
    {
        if (!count) return E_INVALIDARG;
        *count = static_cast<UINT>(m_sources.size());
        return S_OK;
    }

private:
    struct Prop
    {
        std::wstring name;
        winrt::Windows::Foundation::IPropertyValue value;
    };

    GUID m_id;
    winrt::hstring m_name;
    std::vector<Prop> m_props;
    std::vector<wge::IGraphicsEffectSource> m_sources;
};

// Auxiliares de valores de propriedade.
inline winrt::Windows::Foundation::IPropertyValue PropFloat(float v)
{
    return winrt::Windows::Foundation::PropertyValue::CreateSingle(v).as<winrt::Windows::Foundation::IPropertyValue>();
}

inline winrt::Windows::Foundation::IPropertyValue PropUInt32(uint32_t v)
{
    return winrt::Windows::Foundation::PropertyValue::CreateUInt32(v)
        .as<winrt::Windows::Foundation::IPropertyValue>();
}

inline winrt::Windows::Foundation::IPropertyValue PropColor(uint8_t r, uint8_t g, uint8_t b, uint8_t a)
{
    return winrt::Windows::Foundation::PropertyValue::CreateSingleArray(
               {r / 255.0f, g / 255.0f, b / 255.0f, a / 255.0f})
        .as<winrt::Windows::Foundation::IPropertyValue>();
}

}  // namespace tws
