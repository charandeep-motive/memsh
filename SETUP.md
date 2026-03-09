# memsh Setup

This file describes how to ship memsh and how to install it on another machine.

The preferred install path is now a single script:

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

## What needs to be shipped

memsh needs these runtime files on the target machine:

1. the `memsh` binary
2. the zsh plugin at `memsh.zsh`
3. the installer script `install.sh`
4. the uninstaller script `uninstall.sh`

At runtime, memsh writes its database to:

- `$XDG_DATA_HOME/memsh/memsh.db`, or
- `~/.local/share/memsh/memsh.db` when `XDG_DATA_HOME` is not set

## Prerequisites

### Build machine

Use a macOS machine with:

- Go installed
- zsh available

Optional:

- `fzf` for the nicer interactive picker UI

### Target machine

Use a macOS machine with:

- zsh
- write access to `~/.local/bin`
- write access to `~/.config/memsh`

Optional:

- `fzf`

Install `fzf` on macOS with Homebrew if needed:

```sh
brew install fzf
```

## Option 1: Install From Source On Another Machine

This is the simplest path if the other machine also has Go installed.

### Step 1: copy or clone the repo

```sh
git clone <your-repo-url> memsh
cd memsh
```

### Step 2: run the installer

```sh
./install.sh
```

This installs:

- binary: `~/.local/bin/memsh`
- plugin: `~/.config/memsh/memsh.zsh`

The installer:

- builds the binary if needed
- installs `memsh` into `~/.local/bin`
- installs `memsh.zsh` into `~/.config/memsh`
- adds PATH setup to `~/.zshrc` if missing
- adds the memsh plugin source line to `~/.zshrc` if missing

If you want more or fewer suggestions than the default 5, add this before the `source ~/.config/memsh/memsh.zsh` line in `~/.zshrc`:

```sh
export MEMSH_MAX_SUGGESTIONS=10
```

### Step 3: reload zsh

```sh
source ~/.zshrc
```

### Step 4: verify install

```sh
memsh help
memsh doctor
```

If both commands work, the install is complete.

## Option 2: Ship A Built Artifact To Another Machine

This is the better path if the target machine should not need Go.

### Step 1: build on the source machine

From the repo root:

```sh
make build
```

This creates:

```text
bin/memsh
```

### Step 2: package the binary, plugin, and installer

Create a small release folder:

```sh
mkdir -p dist/memsh
cp bin/memsh dist/memsh/
cp shell/memsh.zsh dist/memsh/
cp install.sh dist/memsh/
cp uninstall.sh dist/memsh/
cp README.md dist/memsh/
cp SETUP.md dist/memsh/
```

Archive it:

```sh
tar -czf memsh-darwin-arm64.tar.gz -C dist memsh
```

If you want, rename the archive to include a version, for example:

```sh
mv memsh-darwin-arm64.tar.gz memsh-0.1.0-darwin-arm64.tar.gz
```

### Step 3: transfer the archive to the target machine

Use any file transfer method you prefer, for example:

```sh
scp memsh-0.1.0-darwin-arm64.tar.gz user@target-machine:~
```

### Step 4: unpack on the target machine

```sh
cd ~
tar -xzf memsh-0.1.0-darwin-arm64.tar.gz
```

This gives you:

```text
~/memsh/
```

### Step 5: run the installer on the target machine

```sh
cd ~/memsh
chmod +x install.sh
./install.sh
```

### Step 6: reload zsh

```sh
source ~/.zshrc
```

### Step 7: verify install

```sh
memsh help
memsh doctor
```

## Recommended Shipping Flow

For now, use this release process:

1. Run tests locally.
2. Build the binary.
3. Package `bin/memsh`, `shell/memsh.zsh`, and `install.sh` together.
4. Transfer the archive to the target machine.
5. Run `./install.sh` on the target machine.
6. Reload zsh.
7. Verify with `memsh help` and `memsh doctor`.

Exact commands:

```sh
go test ./...
make build
mkdir -p dist/memsh
cp bin/memsh dist/memsh/
cp shell/memsh.zsh dist/memsh/
cp install.sh dist/memsh/
cp uninstall.sh dist/memsh/
cp README.md dist/memsh/
cp SETUP.md dist/memsh/
tar -czf memsh-0.1.0-darwin-arm64.tar.gz -C dist memsh
```

## Installing A New Version On Another Machine

Overwrite the shipped files and rerun the installer:

```sh
chmod +x install.sh
./install.sh
source ~/.zshrc
```

If the CLI changes, verify again:

```sh
memsh help
memsh doctor
```

If you want to reset all stored suggestions without uninstalling memsh:

```sh
memsh clear
```

`memsh clear` asks for `Y/n` confirmation and prunes the least-used 10% of stored commands.

To wipe the whole suggestion database:

```sh
memsh destroy
```

`memsh destroy` asks for `Y/n` confirmation and removes all stored commands.

## Uninstall

To remove memsh from a machine:

```sh
./uninstall.sh
source ~/.zshrc
```

By default this removes:

- `~/.local/bin/memsh`
- `~/.config/memsh/memsh.zsh`
- the memsh lines added to `~/.zshrc`

It preserves stored history by default.

To also remove stored history:

```sh
MEMSH_UNINSTALL_REMOVE_DATA=1 ./uninstall.sh
source ~/.zshrc
```

That removes data from:

- `$XDG_DATA_HOME/memsh`, or
- `~/.local/share/memsh`

## Notes

- The pretty interactive selector uses `fzf` when available.
- Without `fzf`, memsh still works, but the suggestion UI is less polished.
- memsh currently records only successfully executed commands.
- memsh currently targets macOS + zsh only.