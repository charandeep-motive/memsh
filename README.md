# memsh

memsh is a local shell memory engine for macOS + zsh. It records successfully executed commands into a local SQLite database and returns up to 5 ranked suggestions through a zsh completion widget.

## MVP scope

- macOS + zsh only
- local-first storage
- Go CLI + SQLite backend
- top 5 suggestions
- ranking by frequency + recency

## Commands

```text
memsh --help
memsh --delete "kubectl get pods"
memsh record --command "kubectl get pods" --directory "$PWD" --exit-code 0
memsh search --query "kubectl" --limit 5
memsh stats
memsh doctor
```

## Local install

```sh
./install.sh
```

The plugin watches what you type once the buffer reaches 2 characters and shows a lightweight hint when suggestions are available. Press `Ctrl-Space` or the Down arrow to open a styled picker, then use the Up/Down arrows and Enter to select a command.

You can disable automatic suggestions by setting `MEMSH_AUTOSUGGEST=0` before sourcing the plugin.

memsh only saves commands whose exit code is `0`, so failed commands and common typos do not pollute the suggestion database.

## Development

```sh
make test
make build
```

## Data location

memsh stores data in `$XDG_DATA_HOME/memsh` when `XDG_DATA_HOME` is set. Otherwise it falls back to `~/.local/share/memsh`.