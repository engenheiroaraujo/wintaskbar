# Validação do layout adaptativo da Dock

Data: 12/10/2026, sessão remota do usuário, horário America/Cuiaba.

## O que mudou

O Theme declara `DockLayoutGap` em `Taskbar.TaskbarFrame`. O Agent valida a
folga e emite o contrato versão 2. O TAP acompanha o layout, preserva a largura
natural e centraliza a Dock quando há espaço. Quando ela ultrapassa o início
da Tray menos a folga, desloca a superfície para a esquerda. A largura só é
reduzida pelo excesso medido se a superfície inteira não couber. Isso evita
descontar duas vezes a reserva que o Windows já faz para a Tray.

`DockTrayGap` continua em 12 pixels lógicos. A margem horizontal fixa que
deslocava a Dock mesmo em tela larga foi retirada; `VerticalGap`, cores,
blur, bordas e Mostrar área de trabalho foram preservados.

## Arquivos

- `internal/modules/theme/model/{theme,values,resolve}.go` e `model_test.go`:
  tipo de Style, validação do alvo/valor e testes.
- `internal/modules/theme/repository/{compiled.go,default.yaml,repository_test.go,testdata/default.compiled}`:
  contrato v2, ativação no Default Theme e referência compartilhada.
- `tap/{dock_geometry.h,dock_layout.h,styler.h,tap.cpp}`:
  cálculo, acompanhamento XAML e limpeza de handlers.
- `tap/{theme_file.h,theme_file_test.cpp}`: leitura das versões 1/2 e testes.
- `scripts/measure-taskbar.ps1`: medição por UI Automation, sem ações na UI.
- `README.md`, `CONTEXT.md`, ADRs 0001, 0008 e 0010: comportamento,
  vocabulário, contrato e substituição da margem fixa.

## Verificações automatizadas

- `gofmt -l cmd internal`: saída vazia.
- `go vet ./...`: passou.
- `go test ./...`: passou, também com `WINTASKBAR_TEST_TAP` apontando à DLL nova.
- `scripts/build-tap.ps1 -Out bin/release/tap.dll`: passou.
- `scripts/test-tap.ps1`: passou. Inclui contrato compartilhado, versão antiga,
  rejeição de regras inválidas, folga fracionária, posição, largura e retorno
  ao limite de tela larga; verifica que a reserva da Tray não é descontada duas vezes.
- `scripts/build-release.ps1`: passou; instalador e ZIP gerados em `bin/`.
- ZIP conferido: `wintaskbar.exe`, `tap.dll`, `README.md` e `LICENSE`.
- Verificação de whitespace passou, desconsiderando tabulações finais que
  representam campos vazios intencionais no contrato.

## Medições da revisão final

UI Automation via PowerShell, sem screenshots. O usuário ajustou a largura
do visualizador e confirmou a manutenção dos apps fixados. A quantidade de
elementos de apps visíveis muda quando o Windows os leva ao overflow; não
representa uma alteração do conjunto de apps fixados.

Todas as coordenadas abaixo estão em pixels físicos. A origem da Taskbar foi
`x = 0`. O primeiro botão normal da Tray e Mostrar área de trabalho foram
medidos, além do quadro, `OverflowButton` e `TrayNotifyWnd`.

| Largura da Taskbar | Quadro (x..fim) | Largura da Dock | Overflow (x..fim) | Tray.x | Folga do overflow | Primeiro botão normal (x..fim) | Mostrar área de trabalho (x..fim) |
|---|---|---|---|---|---|---|---|
| 876 | 80..625 | 545 | 573..620 | 637 | 17 | 649..681 | 852..864 |
| 1154 | 50..903 | 853 | 851..898 | 915 | 17 | 927..959 | 1130..1142 |
| 1346 | 208..1095 | 887 | Ausente | 1107 | Não aplicável | 1119..1151 | 1322..1334 |
| 2560 | 837..1724 | 887 | Ausente | 2321 | Não aplicável | 2333..2365 | 2536..2548 |

Nas duas larguras com overflow, `OverflowButton.fim <= TrayNotifyWnd.x`,
com 17 px de folga. A superfície da Dock termina 12 px antes da Tray nas
três larguras com restrição de espaço. Em 1346 px, os 18 elementos de apps
continuam visíveis e somente a posição muda. Em 2560 px, a Dock mantém
887 px, abraça os mesmos 18 elementos e seu centro difere do centro da tela
em apenas 0,5 px. Mostrar área de trabalho permanece visível, com 12 px,
nas quatro medições. Ausência de overflow não foi registrada como teste
aprovado de um botão inexistente.

Registros brutos: [876 px](0010-layout-smallest.json),
[1154 px](0010-layout-narrow.json), [1346 px](0010-layout-medium.json),
[2560 px](0010-layout-wide.json).

A primeira revisão limitava o quadro diretamente ao início da Tray. Embora
evitasse a sobreposição, ela ocultava apps desnecessariamente; foi corrigida
antes das medições acima. Resultados dessa revisão intermediária não são
usados como evidência da implementação final.

## Não verificado e limitações

- Não foi alterado o DPI durante a sessão; conversão e folga fracionária têm
  cobertura de cálculo, mas a transição de DPI real não foi medida.
- Não foram testados monitores secundários, outras versões do Windows,
  inserção/remoção de ícones da Tray nem uso prolongado para medir memória
  e confirmar revogação de handlers sob todas as recriações da árvore.
- A interação de clique no overflow não foi automatizada; verificaram-se
  posição, largura e `IsOffscreen = false`, sem sobreposição com a Tray.
- Sem associação inequívoca entre raiz XAML, Taskbar e `TrayNotifyWnd`, ou
  com transformação personalizada no quadro, a política não é aplicada e
  há registro no log. Não se promete adaptação para árvores desconhecidas.

O TAP foi aplicado com autorização do usuário. Não houve alteração de registro,
auto-ocultar ou apps fixados. Nenhum tema temporário em `%APPDATA%` foi criado.
O Agent não foi iniciado após os testes; o TAP aplicado continua nesta sessão
do Explorer. Não foi criado commit.
