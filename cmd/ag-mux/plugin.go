package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// pluginFile is the plugin shipped with ag-mux, and the name it keeps once
// installed so a later -uninstall can find it again.
const pluginFile = "ag-mux.js"

func runPlugin(args []string) error {
	fs := flag.NewFlagSet("plugin", flag.ExitOnError)
	install := fs.Bool("install", false, "instala o plugin do opencode")
	uninstall := fs.Bool("uninstall", false, "remove o plugin do opencode")
	dir := fs.String("dir", defaultPluginDir(), "diretório de plugins do opencode")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *install && *uninstall {
		return fmt.Errorf("-install e -uninstall não combinam")
	}

	src, err := pluginSource()
	if err != nil {
		return err
	}
	dst := filepath.Join(*dir, pluginFile)

	switch {
	case *install:
		body, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(*dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return err
		}
		fmt.Printf("instalado em %s\n", dst)
		fmt.Println("abra um opencode novo pra valer; os já rodando seguem sem ele.")
		return nil

	case *uninstall:
		if err := os.Remove(dst); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("%s não existe, nada a remover\n", dst)
				return nil
			}
			return err
		}
		fmt.Printf("removido %s\n", dst)
		return nil
	}

	fmt.Printf(`O opencode carrega plugins deste diretório sozinho, sem config:

  %s

O plugin informa duas coisas que não dá pra saber de fora do processo:

  qual sessão é deste pane   o banco amarra sessão a diretório, nunca a
                             terminal, então dois opencode no mesmo diretório
                             hoje aparecem como "?"
  permissão pendente         a tabela de permissões guarda as concedidas; o
                             pedido em aberto só existe como evento

Ele escreve em duas opções do pane (%s e %s), que a sidebar
já lê de graça no list-panes que roda de qualquer jeito.

  ag-mux plugin -install
`, *dir, "@ag-mux-session", "@ag-mux-hook")
	return nil
}

func defaultPluginDir() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return filepath.Join(base, "opencode", "plugin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "plugin"
	}
	return filepath.Join(home, ".config", "opencode", "plugin")
}

// pluginSource finds the plugin shipped next to the binary.
func pluginSource() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	p := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "plugins", "opencode", pluginFile))
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("não achei %s (esperado em %s) — rode 'make build' no repositório", pluginFile, p)
	}
	return p, nil
}
