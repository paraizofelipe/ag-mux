# ag-mux

Uma sidebar de tmux que lista os agentes de codificação rodando na sessão, mostra
qual está trabalhando e qual parou esperando você, e pula pra ele.

```
AGENTES                             1! 3
────────────────────────────────────────
▸✳ proj                                ●
   implementar filtros            12m16s
   ⎇ main
────────────────────────────────────────
 ✳ wt                                  ⣻
   revisar exportacao              8m03s
   ⎇ recurso-novo             ⧉ worktree
────────────────────────────────────────
 π pomar                               ⣻
   escrever testes de integracao     26s
   ⎇ main
────────────────────────────────────────
⏎ ir · z zoom · p pin · x kill · n novo
q sair
```

`1! 3` = 3 agentes, 1 precisa de você.
`⣾` trabalhando · `●` precisa de você · `○` ocioso.
`⎇` a branch (com `*` se houver mudanças não commitadas) · `⧉` é um worktree ligado,
não o checkout principal.

## Harnesses reconhecidos

| Harness | Como é detectado |
|---|---|
| Claude Code | título do pane começa com `✳`, processo em primeiro plano é a versão (`2.1.269`) |
| Oh My Pi (`omp`) | título do pane começa com `π` |

## Instalação

Precisa de Go 1.24+.

```sh
git clone https://github.com/paraizofelipe/ag-mux ~/projects/ag-mux
cd ~/projects/ag-mux && make build
```

No `~/.tmux.conf`:

```tmux
run-shell ~/projects/ag-mux/ag-mux.tmux
```

Recarregue com `tmux source-file ~/.tmux.conf`. Abra a sidebar com `prefix + a`.

<details>
<summary>Com TPM</summary>

```tmux
set -g @plugin 'paraizofelipe/ag-mux'
```

O TPM não compila o binário: rode `make build` no diretório do plugin depois de
instalar.
</details>

## Teclas

| Tecla | Ação |
|---|---|
| `j` `k` `↑` `↓` | navegar |
| `g` `G` | primeiro / último |
| `⏎` | pular pro pane do agente |
| `z` | pular e dar zoom |
| `p` | fixar no topo da lista |
| `x` | matar o agente (pede `y` pra confirmar) |
| `n` | novo agente (`c` = claude, `o` = omp) no diretório do selecionado |
| `r` | atualizar agora |
| `q` | fechar a sidebar |

`prefix + a` mostra e oculta, sempre — não importa qual pane está em foco.

Ocultar fecha o pane em vez de estacioná-lo em algum canto: os pins e a posição
do cursor ficam guardados numa opção do tmux, então a sidebar volta exatamente
como você deixou, sem nenhum processo vivo nem sessão extra no `tmux ls`
enquanto está oculta.

Depois de `⏎` pular pro agente, voltar pra lista é `prefix + a` duas vezes
(oculta e mostra), ou um clique, já que o mouse está ligado.

### Ela aparece em todas as janelas

Enquanto está visível, a sidebar **te acompanha**: ao trocar de janela, ela se
move pra janela que você passou a ver. É uma sidebar só, um processo só, em
todas as janelas.

Isso não é preguiça de implementação — é o limite do tmux. Um pane pertence a
exatamente uma janela: `join-pane` **move** um pane, nunca copia, e não existe
pane compartilhado. As alternativas seriam um pane por janela (N processos
consultando o tmux) ou um pane por janela conversando com um daemon central
(que é o que o [workmux](https://github.com/raine/workmux) faz). Um pane que
migra dá o mesmo resultado visível com um processo e nenhum IPC.

Duas consequências que vale saber:

- Se a janela de destino tiver um **pane em zoom**, a sidebar não entra —
  juntar um pane desfaz o zoom, e zoom significa "quero este pane inteiro".
  Ela migra sozinha assim que você desfaz o zoom, ou entra na hora com
  `prefix + a`. Esse é o único caso em que ela fica numa janela que você não
  está vendo; quando isso acontece ela reduz a varredura a um décimo do ritmo,
  o que na prática mediu 0,03s de CPU contra 0,10s visível.
- Com **dois terminais attachados** na mesma sessão vendo janelas diferentes,
  a sidebar só pode estar em uma delas.

Desligue com `set -g @ag-mux-follow 'off'` se preferir que ela fique na janela
onde você a abriu.

### Ela não segura a janela viva

Quando você fecha o último pane de verdade de uma janela, a sidebar **sai de
lá** e a janela vai embora — como iria se ela não estivesse ali. Sem isso,
sobraria uma janela contendo só a sidebar, que o tmux mantém viva e você teria
que fechar de novo.

Repare que ela se muda, não fecha: como existe uma sidebar só no servidor
inteiro, fechá-la ali a fecharia em todo lugar, inclusive nas janelas em que
você ainda está trabalhando. Ela vai pra janela que você está vendo, ou pra
outra que ainda tenha panes de verdade, preferindo uma que não esteja em zoom.

Só quando não sobra nenhuma janela pra onde ir é que ela fecha de vez — aí a
sessão acabou mesmo, e o tmux encerra como encerraria normalmente.

Fechar um pane quando ainda há outros não mexe em nada.

## Opções

```tmux
set -g @ag-mux-key           'a'    # atalho (usado com o prefix)
set -g @ag-mux-width         '40'   # largura da sidebar em colunas
set -g @ag-mux-interval      '1000' # ms entre varreduras enquanto algo trabalha
set -g @ag-mux-follow        'on'   # 'off' prende a sidebar à janela onde abriu
```

O plugin também escreve `@ag-mux-state` — os pins e o cursor, para sobreviverem
a ocultar/mostrar. É gerenciado sozinho; não precisa mexer.

## Como a detecção funciona

Três estágios, do mais barato pro mais caro:

1. **Candidato** — o título do pane tem a marca do harness. Sai de graça do
   `list-panes` que a sidebar já faz.
2. **Vivo** — o processo em primeiro plano não é um shell, **e** a interface do
   harness está no rodapé da tela. Os dois testes existem porque um harness que
   sai deixa o título pra trás: um pane com título `π - pomar` rodando `zsh` com
   a UI do OMP no scrollback é um agente morto, e uma regra que varre o buffer
   inteiro em vez do rodapé cai nessa.
3. **Estado** — o adapter lê a tela. No Claude Code, a linha de spinner
   (`✢ Wandering… (8m 3s · ↓ 28.4k tokens)`) só existe enquanto ele trabalha, e
   já traz o tempo decorrido; a forma no passado (`✻ Cooked for 1m 27s`) é o que
   ele deixa ao terminar. Texto na caixa de input é um rascunho que você começou
   e não enviou, o que conta como "precisa de você".

Quando nenhuma regra casa, o estado aparece como `?` em vez de um palpite.

## Hooks do Claude Code

Ler a tela tem um limite duro: um diálogo de permissão e uma caixa de input
ociosa são a mesma coisa — um chevron dentro de uma moldura. É por isso que um
agente travado pedindo permissão podia aparecer como ocioso.

O Claude Code sabe a resposta e pode dizer. `ag-mux hook -install` liga os
hooks dele:

```sh
ag-mux hook             # mostra o que seria instalado, pra você conferir antes
ag-mux hook -install    # mescla em ~/.claude/settings.json (com backup)
ag-mux hook -uninstall  # desfaz
```

| Evento | Vira |
|---|---|
| `SessionStart` | ocioso |
| `UserPromptSubmit` | trabalhando |
| `PermissionRequest` | **precisa de você** |
| `Notification` (`permission_prompt`, `idle_prompt`, `agent_needs_input`) | **precisa de você** |
| `Stop`, `StopFailure` | ocioso |
| `SessionEnd` | limpa |

O hook escreve numa **opção do pane**, `@ag-mux-hook`, e não num arquivo nem
num socket. Isso não é economia de código: a sidebar já roda um `list-panes`
por varredura, e ler mais um campo ali custa zero — o estado chega junto com o
resto, sem processo novo. E uma opção de pane morre com o pane, então não há
arquivo órfão nem estado obsoleto pra expirar.

Nada disso substitui a leitura de tela, porque as duas fontes falham de
maneiras opostas. O hook só fala nos eventos em que está ligado: um `ESC` no
meio de um turno, ou um crash, deixam a última palavra dele valendo sem
ninguém pra corrigir. A tela é redesenhada a cada frame, então não envelhece —
mas não distingue permissão de ociosidade.

Então cada uma decide onde é forte: **o hook manda em "precisa de você"**, e a
tela manda no resto — inclusive desfazendo um "precisa de você" vencido assim
que o spinner volta, que é exatamente o que acontece no instante em que você
responde. `ag-mux doctor` mostra as duas leituras e qual ganhou:

```
      3. tela: ocioso
      4. hook: precisa de você  há 4m0s  (permissão)
      → estado: precisa de você  há 4m0s  (permissão)  [hook]
```

De quebra, o hook traz um número que a tela não tem: **há quanto tempo** o
agente está travado. É a diferença entre "alguém precisa de mim" e "alguém
está parado há 12 minutos e eu não vi".

O script é `hooks/ag-mux-hook.sh`, quatro linhas de `sh`. Ele nunca escreve na
saída e sai com 0 em todo caminho — um hook que erra é um harness que engasga.

## Testar

### Sandbox, sem tocar na sua configuração

```sh
make demo
```

Sobe uma sessão `ag-mux-demo` com agentes falsos em estados conhecidos —
trabalhando, precisando de você, ocioso — mais um título obsoleto que **não**
deve aparecer na lista. Os falsos são reais o suficiente para a detecção: cada
pane imprime uma captura de tela de um harness de verdade e estaciona num
processo que não é shell, que é exatamente o que os dois testes de vida olham.

```sh
tmux switch-client -t ag-mux-demo   # de dentro do tmux
tmux attach -t ag-mux-demo          # de fora
```

Lá dentro, `prefix + a`. Para limpar: `make demo-clean`.

### No seu tmux, sem editar arquivo nenhum

```sh
tmux run-shell ~/projects/ag-mux/ag-mux.tmux
```

Registra o atalho só no servidor tmux em execução. Desfaz com `tmux unbind a`,
ou reiniciando o servidor.

### Permanente

A linha `run-shell` no `~/.tmux.conf`, como na instalação acima.

## Branch e worktree

A terceira linha de cada agente mostra em que branch ele está e se aquele
diretório é um worktree ligado (`git worktree add`) em vez do checkout
principal. Com vários agentes no mesmo repositório, é o que diz qual é qual.

A branch sai de graça quando o harness já a imprime: o Claude Code mostra
`git:(main*)` no próprio rodapé, que é a mesma linha que a sidebar já lê para
confirmar que ele está vivo. Quando o harness não mostra — o OMP é o caso —, a
sidebar pergunta ao git.

Essa é a única parte que custa processos, então ela é cacheada: uma invocação
de `git rev-parse` responde branch e worktree de uma vez, e o resultado vale
10 segundos. Sem cache seriam ~11 ms por agente por varredura, mais que todo o
resto da sidebar somado.

## Calibrar

As regras leem a interface do Claude Code e do OMP, então uma atualização deles
pode quebrá-las. `ag-mux doctor` mostra exatamente o que cada estágio decidiu:

```sh
ag-mux doctor            # sessão atual
ag-mux doctor -all       # todas as sessões
ag-mux doctor -all -tail # com as linhas que as regras leram
```

Os adapters ficam em `internal/agent/claude.go` e `internal/agent/omp.go`, um
arquivo cada. Os testes rodam sobre capturas reais em
`internal/agent/testdata/`, sem precisar de tmux:

```sh
make test
```

Pra corrigir uma regra: capture o estado novo com
`tmux capture-pane -p -t %ID -S -45 > internal/agent/testdata/novo-caso.txt`,
adicione uma linha em `fixtures` no `agent_test.go` com o estado esperado, e
ajuste o adapter até passar.

## Limitações conhecidas

- Lista só a sessão atual, por decisão de projeto. Agentes em outras sessões não
  aparecem. (Se a sidebar for levada pra outra sessão, ela passa a listar a
  daquela sessão.)
- Só pode estar em uma janela por vez: com dois clientes tmux vendo janelas
  diferentes, ela aparece em apenas um.
- Sem os hooks instalados, o diálogo de permissão do Claude Code é detectado
  por melhor esforço: a regra foi escrita sem uma captura real do estado. Se um
  agente travado numa pergunta aparecer como ocioso, `ag-mux hook -install`
  resolve de vez; senão, é essa regra que precisa de calibração.
- O OMP distingue trabalhando de ocioso, mas não "esperando resposta sua" —
  ambos aparecem como ocioso.
