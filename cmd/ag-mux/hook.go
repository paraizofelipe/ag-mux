package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookScript is the script Claude Code runs. It is found next to the binary so
// the installed settings keep working wherever the repository lives.
const hookScript = "ag-mux-hook.sh"

// hookEvents maps a Claude Code lifecycle event to the state it means.
//
// Only per-turn and per-session events are wired. PostToolUse would also say
// "working", and it is tempting because it is what unblocks a permission
// dialog — but it fires on every single tool call, and the screen already
// reports work unambiguously through the spinner. Arbitration uses that
// instead, which keeps the hook to a handful of runs per turn.
var hookEvents = []struct {
	Event   string
	Matcher string // "" means every occurrence
	Args    []string
	Why     string
}{
	{"SessionStart", "", []string{"idle"}, "o agente subiu e está com você"},
	{"UserPromptSubmit", "", []string{"busy"}, "você mandou uma tarefa"},
	{"PermissionRequest", "", []string{"waiting", "permissão"}, "travado pedindo permissão"},
	{"Notification", "permission_prompt|idle_prompt|agent_needs_input",
		[]string{"waiting", "entrada"}, "travado esperando você"},
	{"Stop", "", []string{"idle"}, "terminou o turno"},
	{"StopFailure", "", []string{"idle"}, "o turno morreu num erro de API"},
	{"SessionEnd", "", []string{"clear"}, "o agente saiu"},
}

func runHook(args []string) error {
	fs := flag.NewFlagSet("hook", flag.ExitOnError)
	install := fs.Bool("install", false, "escreve os hooks no settings.json do Claude Code")
	uninstall := fs.Bool("uninstall", false, "remove os hooks do settings.json")
	path := fs.String("settings", defaultSettingsPath(), "qual settings.json usar")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *install && *uninstall {
		return fmt.Errorf("-install e -uninstall não combinam")
	}

	script, err := hookScriptPath()
	if err != nil {
		return err
	}

	switch {
	case *install:
		return writeSettings(*path, script, true)
	case *uninstall:
		return writeSettings(*path, script, false)
	default:
		return printSettings(script, *path)
	}
}

// hookScriptPath finds hooks/ag-mux-hook.sh next to the binary and checks it
// can actually run: an entry in settings.json pointing at a missing or
// unexecutable file fails silently on every turn.
func hookScriptPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	p := filepath.Join(filepath.Dir(exe), "..", "hooks", hookScript)
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("não achei %s (esperado em %s) — rode 'make build' no repositório", hookScript, p)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s não é executável: rode 'chmod +x %s'", hookScript, p)
	}
	return p, nil
}

func defaultSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "settings.json"
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// hookBlock is the "hooks" object to merge into settings.json.
func hookBlock(script string) map[string]any {
	block := map[string]any{}
	for _, e := range hookEvents {
		handler := map[string]any{
			"type":    "command",
			"command": script,
		}
		// args puts Claude Code in exec form: no shell, so each argument
		// arrives verbatim and nothing has to be quoted.
		handler["args"] = toAny(e.Args)

		group := map[string]any{"hooks": []any{handler}}
		if e.Matcher != "" {
			group["matcher"] = e.Matcher
		}
		block[e.Event] = []any{group}
	}
	return block
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func printSettings(script, path string) error {
	fmt.Printf("O Claude Code avisa a sidebar chamando este script a cada evento:\n\n  %s\n\n", script)
	fmt.Println("eventos ligados:")
	for _, e := range hookEvents {
		fmt.Printf("  %-18s → %-8s %s\n", e.Event, e.Args[0], e.Why)
	}
	out, err := json.MarshalIndent(map[string]any{"hooks": hookBlock(script)}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("\ncole isto em %s:\n\n%s\n\n", path, out)
	fmt.Println("ou deixe o ag-mux mesclar, preservando o resto do arquivo:")
	fmt.Println("  ag-mux hook -install")
	return nil
}

// writeSettings merges our entries into settings.json, or removes them.
//
// It never rewrites entries it did not create: every event list is filtered of
// handlers pointing at our script and then, when installing, ours is appended.
// So an event you also hook for something else keeps your handler.
func writeSettings(path, script string, install bool) error {
	settings := map[string]any{}
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &settings); err != nil {
			return fmt.Errorf("%s não é JSON válido, não vou mexer nele: %w", path, err)
		}
	case os.IsNotExist(err):
		if !install {
			fmt.Printf("%s não existe, nada a remover\n", path)
			return nil
		}
	default:
		return err
	}

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}

	removed := 0
	for _, e := range hookEvents {
		groups, _ := hooks[e.Event].([]any)
		kept := make([]any, 0, len(groups))
		for _, g := range groups {
			before := handlerCount(g)
			g, after := withoutScript(g, script)
			removed += before - after
			if after > 0 {
				kept = append(kept, g)
			}
		}
		if install {
			block := hookBlock(script)[e.Event].([]any)
			kept = append(kept, block...)
		}
		if len(kept) == 0 {
			delete(hooks, e.Event)
			continue
		}
		hooks[e.Event] = kept
	}

	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	if raw != nil {
		backup := path + ".bak"
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return fmt.Errorf("não consegui salvar o backup %s: %w", backup, err)
		}
		fmt.Printf("backup: %s\n", backup)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return err
	}

	if install {
		fmt.Printf("instalado em %s (%d evento(s))\n", path, len(hookEvents))
		if removed > 0 {
			fmt.Printf("substituí %d entrada(s) antiga(s) do ag-mux\n", removed)
		}
		fmt.Println("abra um Claude Code novo pra valer; os já rodando seguem com a config antiga.")
	} else {
		fmt.Printf("removido de %s (%d entrada(s))\n", path, removed)
	}
	return nil
}

// withoutScript drops the handlers in a matcher group that point at our script,
// returning the group and how many handlers are left.
func withoutScript(group any, script string) (any, int) {
	g, ok := group.(map[string]any)
	if !ok {
		return group, 1 // not ours to understand; leave it alone
	}
	handlers, ok := g["hooks"].([]any)
	if !ok {
		return group, 1
	}
	kept := make([]any, 0, len(handlers))
	for _, h := range handlers {
		if isOurHandler(h, script) {
			continue
		}
		kept = append(kept, h)
	}
	g["hooks"] = kept
	return g, len(kept)
}

func isOurHandler(handler any, script string) bool {
	h, ok := handler.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := h["command"].(string)
	// Match by script name, not by full path: a repository that moved should
	// still be recognised as ours instead of leaving a dead entry behind.
	return cmd == script || strings.Contains(cmd, hookScript)
}

func handlerCount(group any) int {
	g, ok := group.(map[string]any)
	if !ok {
		return 1
	}
	handlers, ok := g["hooks"].([]any)
	if !ok {
		return 1
	}
	return len(handlers)
}
