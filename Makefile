# Windows x64 + GNU Make. Native builds and packaging use the existing scripts.
SHELL := cmd.exe
.SHELLFLAGS := /D /C
.DEFAULT_GOAL := help
.NOTPARALLEL:

GO ?= go
POWERSHELL ?= powershell.exe
PS = "$(POWERSHELL)" -NoProfile -NonInteractive

export GOOS := windows
export GOARCH := amd64

.PHONY: help all build agent cli tap release test test-go test-tap test-ui vet check fmt run stop clean

help:
	@$(PS) -Command "Write-Output 'wintaskbar - Windows x64 / GNU Make' '' '  make build     Compila o aplicativo, o TAP e a CLI' '  make release   Gera o instalador e o ZIP para uso sem instalar' '  make test      Executa os testes Go/COM e de contrato C++' '  make test-ui   Testa a bandeja sem aplicar um tema' '  make check     Executa todos os testes e o go vet' '  make fmt       Formata os arquivos Go' '  make run       Compila e abre o aplicativo (aplica o tema)' '  make stop      Restaura a barra de tarefas e fecha o aplicativo' '  make clean     Remove os arquivos gerados' '' '  Alvos individuais: agent, cli, tap, test-go, test-tap, vet'"

all: build

build: tap agent cli

agent:
	@$(PS) -Command "New-Item -ItemType Directory -Force -Path 'bin/release' | Out-Null"
	"$(GO)" build -trimpath -ldflags "-H=windowsgui -s -w" -o bin/release/wintaskbar.exe ./cmd/wintaskbar

cli:
	@$(PS) -Command "New-Item -ItemType Directory -Force -Path 'bin' | Out-Null"
	"$(GO)" build -trimpath -o bin/wintaskbar-cli.exe ./cmd/wintaskbar

tap:
	$(PS) -File scripts/build-tap.ps1 -Out bin/release/tap.dll

release: tap
	$(PS) -File scripts/build-release.ps1 -SkipTap

test: test-go test-tap

# Test the freshly built DLL; UI tests require the explicit test-ui target.
test-go: tap
	$(PS) -Command "$$env:WINTASKBAR_TEST_TAP = (Resolve-Path 'bin/release/tap.dll').Path; $$env:WINTASKBAR_TEST_TRAY = '0'; & '$(GO)' test ./...; exit $$LASTEXITCODE"

test-tap:
	$(PS) -File scripts/test-tap.ps1

test-ui:
	$(PS) -Command "$$env:WINTASKBAR_TEST_TRAY = '1'; & '$(GO)' test -count=1 ./internal/platform/windows/tray; exit $$LASTEXITCODE"

vet:
	"$(GO)" vet ./...

# Gera o conteúdo do instalador antes de testar, liga as verificações nativas de
# UI e ignora o cache de testes do Go, para este alvo executar a suíte inteira.
check:
	$(PS) -File scripts/build-release.ps1 -OutDir bin/check
	$(PS) -File scripts/test-tap.ps1
	"$(GO)" vet ./...
	$(PS) -Command "$$env:WINTASKBAR_TEST_TAP = (Resolve-Path 'bin/check/release/tap.dll').Path; $$env:WINTASKBAR_TEST_TRAY = '1'; & '$(GO)' test -count=1 ./...; exit $$LASTEXITCODE"

fmt:
	gofmt -w cmd internal

run: build
	$(PS) -Command "Start-Process -FilePath (Resolve-Path 'bin/release/wintaskbar.exe').Path -WindowStyle Hidden"

stop: cli
	bin\wintaskbar-cli.exe stop

clean:
	$(PS) -File scripts/clean-build.ps1
