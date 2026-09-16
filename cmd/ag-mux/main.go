// Command ag-mux lists the coding agents running in a tmux session.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/paraizofelipe/ag-mux/internal/agent"
	"github.com/paraizofelipe/ag-mux/internal/tmux"
)

const version = "0.1.0"

const usage = `ag-mux — sidebar de agentes para tmux

uso:
  ag-mux sidebar   roda a sidebar (chamado pelo tmux, não à mão)
  ag-mux toggle    mostra ou oculta a sidebar
  ag-mux follow    traz a sidebar pra janela atual (chamado por hook)
  ag-mux reap      fecha janelas que só têm a sidebar (chamado por hook)
  ag-mux doctor    mostra o que a detecção enxerga em cada pane
  ag-mux version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "sidebar":
		err = runSidebar(os.Args[2:])
	case "toggle":
		err = runToggle(os.Args[2:])
	case "follow":
		err = runFollow(os.Args[2:])
	case "reap":
		err = runReap(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ag-mux:", err)
		os.Exit(1)
	}
}

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	all := fs.Bool("all", false, "todas as sessões, não só a atual")
	showTail := fs.Bool("tail", false, "imprime as linhas que as regras leram")
	depth := fs.Int("depth", 6, "quantas linhas do rodapé mostrar com -tail")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var (
		panes []tmux.Pane
		err   error
		scope = "todas as sessões"
	)
	if *all {
		panes, err = tmux.ListAllPanes()
	} else {
		var session string
		if session, err = tmux.CurrentSession(); err != nil {
			return fmt.Errorf("não consegui descobrir a sessão atual (está dentro do tmux?): %w", err)
		}
		scope = "sessão " + session
		panes, err = tmux.ListPanes(session)
	}
	if err != nil {
		return err
	}

	fmt.Printf("%d panes, %s\n\n", len(panes), scope)
	live := 0
	for _, p := range panes {
		fmt.Printf("%-5s %s:%d.%d  cmd=%-10s\n", p.ID, p.WindowName, p.WindowIndex, p.Index, p.Command)
		fmt.Printf("      título: %q\n", p.Title)

		if p.IsSidebar {
			fmt.Printf("      → é a própria sidebar, ignorado\n\n")
			continue
		}
		claimed := false
		for _, ad := range agent.Adapters() {
			if !ad.IsCandidate(p) {
				continue
			}
			claimed = true
			fmt.Printf("      1. candidato a %s: sim\n", ad.Name())

			tail, err := tmux.CapturePane(p.ID, agent.CaptureLines)
			if err != nil {
				fmt.Printf("      2. captura falhou: %v\n\n", err)
				break
			}
			ok := ad.Confirm(tail)
			fmt.Printf("      2. vivo: %v", ok)
			if !ok {
				fmt.Printf("  (chrome ausente no rodapé — título obsoleto)\n")
			} else {
				fmt.Println()
				state, detail, elapsed := ad.Classify(p, tail)
				fmt.Printf("      3. estado: %s", state)
				if elapsed > 0 {
					fmt.Printf("  há %s", elapsed)
				}
				if detail != "" {
					fmt.Printf("  (%s)", detail)
				}
				fmt.Printf("\n      tarefa: %q\n", ad.Task(p))
				live++
			}
			if *showTail {
				fmt.Println("      rodapé lido:")
				for _, l := range tailOf(tail, *depth) {
					fmt.Printf("        | %s\n", l)
				}
			}
			break
		}
		if !claimed {
			fmt.Printf("      → nenhum harness reconhecido\n")
		}
		fmt.Println()
	}
	fmt.Printf("%d agente(s) vivo(s)\n", live)
	return nil
}

func tailOf(lines []string, n int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if lines[i] == "" {
			continue
		}
		out = append([]string{lines[i]}, out...)
	}
	return out
}
