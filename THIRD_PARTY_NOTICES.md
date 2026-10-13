# Avisos de terceiros

O wintaskbar é distribuído sob a licença MIT (veja `LICENSE`). O código e o tema
padrão deste repositório são do próprio projeto. Este arquivo lista o que o
projeto usa de terceiros e as licenças correspondentes.

## Biblioteca incluída nos executáveis

### gopkg.in/yaml.v3 (v3.0.1)

Usada pelo Agent para ler os arquivos de tema (YAML).

Esta biblioteca é coberta por duas licenças: MIT e Apache 2.0.

**Licença MIT.** Os arquivos `apic.go`, `emitterc.go`, `parserc.go`,
`readerc.go`, `scannerc.go`, `writerc.go`, `yamlh.go` e `yamlprivateh.go` foram
portados para Go a partir de arquivos C do libyaml e continuam cobertos pela
licença original:

```text
Copyright (c) 2006-2010 Kirill Simonov
Copyright (c) 2006-2011 Kirill Simonov

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
of the Software, and to permit persons to whom the Software is furnished to do
so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

**Licença Apache 2.0.** Todos os demais arquivos da biblioteca:

```text
Copyright (c) 2011-2019 Canonical Ltd

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

Aviso (NOTICE) da biblioteca: `Copyright 2011-2016 Canonical Ltd.`

## Ferramentas de compilação

A `tap.dll` (C++/WinRT) é compilada com o **clang/LLVM 20** em uma toolchain
llvm-mingw, que traz os cabeçalhos e a biblioteca de execução do **mingw-w64**,
além da biblioteca C++ do LLVM. Os termos estão no `LICENSE.TXT` da própria
toolchain (LLVM é Apache 2.0 com exceção LLVM, que permite distribuir o código
gerado sem exigir atribuição das partes de tempo de execução). A toolchain não é
distribuída com este projeto: fica em `.toolchain/`, fora do controle de versão.

## Interfaces do Windows

O projeto usa APIs do Windows (Win32, Shell, DWM, XAML e a API de diagnóstico
XAML). Os nomes de elementos usados nos seletores do tema (por exemplo
`Taskbar.TaskbarFrame`) são identificadores da plataforma, observados na árvore
visual do explorer.
