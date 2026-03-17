# memsh

memsh is a local shell memory engine for macOS + zsh. It records successfully executed commands into a local SQLite database and returns ranked suggestions through a zsh completion widget.

## MVP scope

- macOS + zsh only
- local-first storage
- Go CLI + SQLite backend
- configurable suggestion count, default 5
- ranking by frequency + recency

## Commands

```text
memsh
memsh help
memsh clear
memsh destroy
memsh delete "kubectl get pods"
memsh record --command "kubectl get pods" --directory "$PWD" --exit-code 0
memsh search --query "kubectl" --limit 5
memsh stats
memsh doctor
```

## Local install

```sh
./install.sh
```

Run `memsh` with no arguments to open an interactive command-search box. Type inside the picker to filter stored commands, then press Enter to print the selected command.

The plugin watches what you type once the buffer reaches 2 characters and shows a lightweight hint when suggestions are available. Press `Ctrl-Space` or the Down arrow to open a styled picker, then use the Up/Down arrows and Enter to select a command.

You can disable automatic suggestions by setting `MEMSH_AUTOSUGGEST=0` before sourcing the plugin.

You can change the number of suggestions shown by setting `MEMSH_MAX_SUGGESTIONS` before sourcing the plugin, for example:

```sh
export MEMSH_MAX_SUGGESTIONS=10
source ~/.config/memsh/memsh.zsh
```

memsh only saves commands whose exit code is `0`, so failed commands and common typos do not pollute the suggestion database.

memsh also skips recording its own administrative commands like `memsh help`, `memsh stats`, `memsh clear`, and `memsh destroy`.

## Development

```sh
make test
make build
```

## Data location

memsh stores data in `$XDG_DATA_HOME/memsh` when `XDG_DATA_HOME` is set. Otherwise it falls back to `~/.local/share/memsh`.