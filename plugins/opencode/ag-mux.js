// ag-mux — o opencode informa o próprio estado para a sidebar.
//
// Instalado em ~/.config/opencode/plugin/, de onde o opencode carrega sozinho:
// não há config para editar. Ele existe porque duas coisas não dá para saber
// de fora do processo:
//
//   1. Qual sessão pertence a este pane. O banco do opencode amarra sessão a
//      diretório, nunca a terminal, então dois opencode no mesmo diretório são
//      indistinguíveis — e a sidebar prefere mostrar "?" a chutar.
//   2. Que ele está parado pedindo permissão. A tabela `permission` guarda
//      permissões concedidas; o pedido pendente só existe como evento.
//
// O que ele escreve são duas opções do pane, que a sidebar já lê de graça no
// list-panes que roda de qualquer jeito. Opção de pane morre com o pane, então
// não há nada para limpar.

const PANE = process.env.TMUX_PANE
const ENABLED = Boolean(process.env.TMUX && PANE)

const SESSION_OPTION = "@ag-mux-session"
const STATE_OPTION = "@ag-mux-hook"

async function tmux(args) {
  // Nunca deixa uma falha do tmux escapar para o opencode: um plugin que
  // lança é um agente que engasga.
  try {
    const { execFile } = await import("node:child_process")
    await new Promise((resolve) => {
      const child = execFile("tmux", args, () => resolve())
      child.on("error", () => resolve())
    })
  } catch {
    /* ignorado de propósito */
  }
}

const setOption = (name, value) => tmux(["set-option", "-p", "-t", PANE, name, value])
const unsetOption = (name) => tmux(["set-option", "-p", "-u", "-t", PANE, name])

const now = () => Math.floor(Date.now() / 1000)

export const AgMux = async () => {
  if (!ENABLED) return {}

  // Sessões filhas são subagentes. O que interessa é a conversa que você
  // abriu, então os eventos delas não roubam o lugar da sessão raiz.
  const parents = new Map()
  let reported = ""
  let pending = 0

  const rootOf = (id) => {
    let root = id
    const seen = new Set()
    while (parents.has(root) && !seen.has(root)) {
      seen.add(root)
      root = parents.get(root)
    }
    return root
  }

  const remember = (id) => {
    if (!id || id === reported) return null
    reported = id
    return setOption(SESSION_OPTION, id)
  }

  const block = (why) => {
    pending += 1
    return setOption(STATE_OPTION, `waiting ${now()} ${why}`)
  }

  const unblock = () => {
    pending = Math.max(0, pending - 1)
    return pending === 0 ? unsetOption(STATE_OPTION) : null
  }

  const clear = () => {
    pending = 0
    return unsetOption(STATE_OPTION)
  }

  return {
    event: async ({ event }) => {
      const type = event?.type
      const props = event?.properties ?? {}
      const info = props.info ?? {}
      if (info.id && info.parentID) parents.set(info.id, info.parentID)

      const id = props.sessionID || info.sessionID || info.id || ""
      if (id && rootOf(id) !== id) return // evento de subagente

      switch (type) {
        case "session.created":
        case "session.updated":
        case "session.status":
        case "message.updated":
          await remember(id)
          break
        case "permission.asked":
          await remember(id)
          await block("permissão")
          break
        case "permission.replied":
          await unblock()
          break
        case "session.idle":
          await clear()
          break
        case "session.deleted":
          if (id && id === reported) {
            reported = ""
            await clear()
            await unsetOption(SESSION_OPTION)
          }
          break
      }
    },
  }
}
