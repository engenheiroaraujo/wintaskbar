//go:build windows

package main

import (
	"bytes"
	"debug/pe"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallLocationIsUserScoped(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\Test User\AppData\Local`)
	dir, err := installDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != `C:\Users\Test User\AppData\Local\Programs\wintaskbar` {
		t.Fatal(dir)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, err := installDir(); err == nil {
		t.Fatal("accepted missing user directory")
	}
}

func TestReplaceFileUpdatesExistingInstallation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.exe")
	if err := replaceFile(path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("%s, %v", data, err)
	}
	if _, err := os.Stat(path + ".new"); !os.IsNotExist(err) {
		t.Fatal("staging file not removed")
	}
}

func TestReleasePayloadIsSelfContainedX64(t *testing.T) {
	if _, err := payload.ReadFile("payload/wintaskbar.exe"); err != nil {
		t.Skip("run scripts/build-release.ps1 first")
	}
	if err := checkPayload(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"wintaskbar.exe", "tap.dll"} {
		data, _ := payload.ReadFile("payload/" + name)
		file, err := pe.NewFile(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if file.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			t.Fatalf("%s is not x64", name)
		}
		if name == "wintaskbar.exe" && file.OptionalHeader.(*pe.OptionalHeader64).Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
			t.Fatal("desktop build would open a console")
		}
		imports, err := file.ImportedSymbols()
		if err != nil {
			t.Fatal(err)
		}
		for _, symbol := range imports {
			lower := strings.ToLower(symbol)
			for _, runtime := range []string{"libgcc", "libstdc++", "libwinpthread", "vcruntime", "msvcp140"} {
				if strings.Contains(lower, runtime) {
					t.Fatalf("%s requires external compiler runtime: %s", name, symbol)
				}
			}
		}
	}
}
