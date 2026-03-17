# memsh Setup

The preferred install path is:

```sh
./install.sh
```

The preferred uninstall path is:

```sh
./uninstall.sh
```

Current scope:

- macOS only
- zsh only
- local SQLite storage
- `fzf` recommended for the best picker UI
- configurable suggestion count through `MEMSH_MAX_SUGGESTIONS`
- `memsh` with no arguments opens an interactive search box

## Prerequisites

### Build machine

Use a macOS machine with:

- Go installed
- zsh available

Optional:

- `fzf` for the nicer interactive picker UI