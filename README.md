# dotfiles

Easily install and use important dotfiles in any project, with easy synchronization

## Installation

Clone the project into a `.bash` folder in your home directory (create the directory if it does not exist)

```sh
cd ~/.bash
git clone git@github.com:wesscoby/dotfiles.git
cd dotfiles
```

## Usage

- Copy the npm config file to your home directory
- Copy your ssh keys into the .ssh directory
  - Add the following to your zsh config
    ```sh
    # Export bin
    export PATH=$PATH:~/.bash/dotfiles/bin
    ```
  - Reload zsh and modify permissions for the ssh keys as follows:
    ```sh
    # Source
    source ~/.zshrc

    # Modify permissions
    sshp
    ```
- Add the following to your zsh config
  ```sh
  # Load defined environment variables
  if [ -f ~/.bash/dotfiles/.env ]; then
  		. ~/.bash/dotfiles/.env;
  fi

  # Load defined aliases
  if [ -f ~/.bash/dotfiles/.aliases ]; then
  		. ~/.bash/dotfiles/.aliases;
  fi

  # Start the ssh-agent
  eval `ssh-agent`

  # Load defined functions
  fpath+=~/.bash/dotfiles/zsh-functions
  autoload p pv gcfg s wt kp activate venv denv lenv nig mpush msh pw

  # Set default ssh keys
  s
  ```
- Reload zsh
  ```sh
  source ~/.zshrc
  ```

## Formatting

Shell files are formatted with [shfmt](https://github.com/mvdan/sh) and markdown with [Prettier](https://prettier.io). `bin/fmt` drives both.

```sh
fmt            # format every tracked file
fmt --check    # list what needs formatting, write nothing
fmt --staged   # format staged files and restage them (used by the hook)
```

A pre-commit hook runs `fmt --staged` on every commit. It is not enabled automatically on a fresh clone — turn it on once per machine:

```sh
git config core.hooksPath hooks
```

Both tools are optional. If either is missing `fmt` warns and skips those files rather than blocking the commit. To install them:

```sh
sudo apt-get install shfmt
npm install -g prettier   # or just run nig
```

Note that shfmt parses zsh functions as bash, which works because none of them use zsh-only syntax. Keep it that way, or add the file to the exclusions in `bin/fmt`.

One sharp edge worth knowing: shfmt treats an unquoted array subscript as arithmetic, so an associative-array key like `[a11-wb]` gets rewritten to `[a11 - wb]` and the lookup silently breaks. Quote associative-array keys (see `pw`). After a formatting change to a function, `zsh -n <file>` catches syntax breakage, and diffing `functions <name>` before and after catches semantic drift.

## Detailed Documentation and Examples

### Aliases

#### Git Aliases

- `gpush`: Push to the origin repository
- `gpull`: Pull from the origin repository
- `dpush`: Push to the development branch
- `dpull`: Pull from the development branch
- `rnm`: Rename the current branch to main
- `gcop`: Checkout the main branch and pull the latest changes

#### NPM Aliases

- `n`: Run npm
- `ni`: Install npm packages
- `nu`: Uninstall npm packages
- `ns`: Start an npm project
- `nr`: Run an npm script
- `nb`: Build an npm project
- `nd`: Run npm in development mode

#### Docker Aliases

- `dk`: Run docker
- `dkc`: Run docker-compose

### Functions

#### `activate`

Activate a Python virtual environment.

```sh
activate <environment_name>
```

#### `denv`

Delete a Python virtual environment.

```sh
denv <environment_name>
```

#### `gcfg`

Show git config current user.

```sh
gcfg
```

#### `kp`

Kill the process running on the specified port.

```sh
kp <port>
```

#### `lenv`

List the available virtual environments.

```sh
lenv
```

#### `mpush`

Upload to remote machine via ssh.

```sh
mpush <machine_name> <file> [remote_path]
```

#### `nig`

Install global npm packages.

```sh
nig
```

#### `p`

Navigate to my projects directory (~/Projects) or any subdirectory.

```sh
p [subdirectory]
```

#### `s`

Switch between the ssh keys in the ssh-agent and their corresponding global username and email in git config.

```sh
s [key_name]
```

#### `venv`

Create a Python virtual environment.

```sh
venv <python_version> <environment_name>
```

#### `wt`

Clone a Wesnetech project based on name using gh (Github CLI).

```sh
wt <repo_name> [directory]
```
