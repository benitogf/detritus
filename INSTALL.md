# Manual Install

This file is for AI tools and maintainers that need explicit shell commands.
Human users should usually ask their AI coding tool to install Detritus from the
GitHub repository instead of running these steps themselves.

## Install From Release Scripts

Linux/macOS:

```bash
curl -fsSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | bash
```

Windows PowerShell:

```powershell
iwr -useb https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

The install scripts download the latest release and run:

```bash
detritus --setup
```

`detritus --setup` configures detected assistant apps and editors.

## Install From A Downloaded Binary

Download a binary from the
[latest release](https://github.com/benitogf/detritus/releases/latest), put it on
`PATH`, then run:

```bash
detritus --setup
```

## Update

```bash
detritus --update
```

`detritus --update` downloads the latest release and runs setup with the new
binary.

## Workspace Pack CLI

Most users should use `/code`. These commands are available for tools and
maintainers that need direct pack management:

```bash
detritus --pack my-project /absolute/path/to/project
detritus --packs
detritus --refresh my-project
detritus --unpack my-project
```
