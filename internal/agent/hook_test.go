package agent

import (
	"testing"
	"time"
)

var hookEpoch = time.Unix(1700000000, 0)

func TestParseHook(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   hookReport
		wantOk bool
	}{
		{"estado e detalhe", "waiting 1700000000 permissão",
			hookReport{StateWaiting, "permissão", hookEpoch}, true},
		{"detalhe com espaços", "waiting 1700000000 permissão de Bash",
			hookReport{StateWaiting, "permissão de Bash", hookEpoch}, true},
		{"sem detalhe", "busy 1700000000",
			hookReport{StateBusy, "", hookEpoch}, true},
		{"detalhe vazio", "idle 1700000000 ",
			hookReport{StateIdle, "", hookEpoch}, true},

		// Anything unparseable must read as "no hook", never as a state: the
		// sidebar then falls back to screen detection instead of breaking.
		{"vazio", "", hookReport{}, false},
		{"opção não definida", "   ", hookReport{}, false},
		{"estado desconhecido", "confused 1700000000", hookReport{}, false},
		{"sem epoch", "waiting", hookReport{}, false},
		{"epoch não numérico", "waiting agora", hookReport{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseHook(c.in)
			if ok != c.wantOk {
				t.Fatalf("ok = %v, queria %v", ok, c.wantOk)
			}
			if ok && got != c.want {
				t.Errorf("got %+v, queria %+v", got, c.want)
			}
		})
	}
}

func TestArbitrate(t *testing.T) {
	now := hookEpoch.Add(5 * time.Minute)
	busy := screenState{StateBusy, "8m 3s", 8*time.Minute + 3*time.Second}
	idle := screenState{StateIdle, "", 0}
	draft := screenState{StateWaiting, "rascunho pendente: oi", 0}
	unknown := screenState{StateUnknown, "", 0}

	hook := func(s State, detail string) hookReport {
		return hookReport{State: s, Detail: detail, At: hookEpoch}
	}

	cases := []struct {
		name       string
		screen     screenState
		hook       hookReport
		hasHook    bool
		want       screenState
		wantSource Source
	}{
		{"sem hook, a tela manda", busy, hookReport{}, false, busy, SourceScreen},
		{"sem hook e sem regra continua ?", unknown, hookReport{}, false, unknown, SourceScreen},

		// The whole point: the screen cannot see a permission dialog, so the
		// hook's word stands and brings the one number the screen lacks —
		// how long it has been blocked.
		{"hook vê permissão, a tela não", idle, hook(StateWaiting, "permissão"), true,
			screenState{StateWaiting, "permissão", 5 * time.Minute}, SourceHook},
		{"permissão sem detalhe", idle, hook(StateWaiting, ""), true,
			screenState{StateWaiting, "pendente", 5 * time.Minute}, SourceHook},
		{"hook vence o rascunho da tela", draft, hook(StateWaiting, "permissão"), true,
			screenState{StateWaiting, "permissão", 5 * time.Minute}, SourceHook},

		// You answered: the spinner is back and the hook has not spoken yet.
		// Without this the sidebar would stay red until the turn ended.
		{"spinner desfaz um waiting vencido", busy, hook(StateWaiting, "permissão"), true,
			busy, SourceScreen},

		// For busy and idle the screen is redrawn every frame; the hook only
		// speaks at events, so it is the one that can be stale.
		{"tela desmente busy obsoleto", idle, hook(StateBusy, ""), true, idle, SourceScreen},
		{"tela desmente idle obsoleto", busy, hook(StateIdle, ""), true, busy, SourceScreen},
		{"tela vence com rascunho", draft, hook(StateIdle, ""), true, draft, SourceScreen},

		// ...except when no rule matched at all, where anything beats "?".
		{"hook preenche o que a tela não sabe", unknown, hook(StateBusy, ""), true,
			screenState{StateBusy, "", 0}, SourceHook},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, src := arbitrate(c.screen, c.hook, c.hasHook, false, now)
			if got != c.want {
				t.Errorf("estado = %+v, queria %+v", got, c.want)
			}
			if src != c.wantSource {
				t.Errorf("fonte = %v, queria %v", src, c.wantSource)
			}
		})
	}
}

// A clock that runs backwards (NTP step, a laptop waking up) must not print a
// negative "esperando há -3s".
func TestArbitrateClampsNegativeWait(t *testing.T) {
	got, _ := arbitrate(
		screenState{StateIdle, "", 0},
		hookReport{State: StateWaiting, At: hookEpoch},
		true,
		false,
		hookEpoch.Add(-time.Minute),
	)
	if got.Elapsed != 0 {
		t.Errorf("elapsed = %v, queria 0", got.Elapsed)
	}
}

// A harness that reports the end of a wait as well as its start must not be
// second-guessed. opencode does: a turn paused on a permission prompt still
// reads as a turn in flight in its database, so without this the sidebar would
// overrule a report that is simply right.
func TestAuthoritativeHookIsNotOverruled(t *testing.T) {
	now := hookEpoch.Add(90 * time.Second)
	busy := screenState{StateBusy, "", time.Minute}
	waiting := hookReport{State: StateWaiting, Detail: "permissão", At: hookEpoch}

	got, src := arbitrate(busy, waiting, true, true, now)
	if got.State != StateWaiting {
		t.Errorf("estado = %v, queria precisa de você", got.State)
	}
	if src != SourceHook {
		t.Errorf("fonte = %v, queria hook", src)
	}
	if got.Elapsed != 90*time.Second {
		t.Errorf("esperando há %v, queria 1m30s", got.Elapsed)
	}

	// Sem essa garantia, a outra fonte manda — que é o certo para um harness
	// que não avisa o fim da espera.
	if got, _ := arbitrate(busy, waiting, true, false, now); got.State != StateBusy {
		t.Errorf("sem autoridade o estado devia vir da outra fonte, veio %v", got.State)
	}
}
