<p align="center">
  <img src="mmcliBanner.png" alt="mmcli banner" />
</p>

# mmcli

A command-line Valheim mod manager for macOS, Linux and Windows. Installs mods from [Thunderstore](https://thunderstore.io/c/valheim/) and [Hexium](https://valheim.hexium.gg/), manages profiles, and launches the game with BepInEx.

This is a fork of [jneb802/mod-manager-cli](https://github.com/jneb802/mod-manager-cli). Credit for the original project goes to [jneb802](https://github.com/jneb802) and its contributors. This fork adds Hexium support, portable profile transfers, and Windows builds and tests, building on the original project's Windows implementation.

## Install

Download this fork's [preview release](https://github.com/mitchieui/mod-manager-cli/releases/tag/v0.1.0-windows-preview.1):

- [Windows (x64)](https://github.com/mitchieui/mod-manager-cli/releases/download/v0.1.0-windows-preview.1/mmcli-windows-amd64.exe)
- [Mac (Apple Silicon)](https://github.com/mitchieui/mod-manager-cli/releases/download/v0.1.0-windows-preview.1/mmcli-darwin-arm64)
- [Mac (Intel)](https://github.com/mitchieui/mod-manager-cli/releases/download/v0.1.0-windows-preview.1/mmcli-darwin-amd64)

These are direct downloads; no GitHub sign-in is needed. Windows unit tests pass, but game launching and mod loading still need gameplay validation.

The shell script and Homebrew instructions below install the original upstream version.

### Shell script (recommended for macOS/Linux)

```
curl -fsSL https://raw.githubusercontent.com/jneb802/mod-manager-cli/main/install.sh | bash
```

The installer uses a user-writable location and does not require a password. It updates an existing writable `mmcli` install, installs to a writable directory already on your `PATH`, or installs to `~/.local/bin`.

If it installs to `~/.local/bin` and your shell cannot find `mmcli`, the installer adds this to your shell profile:

```
export PATH="$HOME/.local/bin:$PATH"
```

Restart your terminal, or run the `source` command printed by the installer.

### Homebrew

```
brew install jneb802/tap/mmcli
```

### Manual download

Download the latest binary for your platform from the original project's [Releases](https://github.com/jneb802/mod-manager-cli/releases), or use this fork's [Mac downloads](https://github.com/mitchieui/mod-manager-cli/releases/tag/v0.1.0-windows-preview.1), then:

```
mkdir -p ~/.local/bin
binary=mmcli-linux-amd64 # or mmcli-darwin-arm64 / mmcli-darwin-amd64
chmod +x "$binary"
mv "$binary" ~/.local/bin/mmcli
```

### Windows

Install Steam and Valheim, then [download the Windows executable](https://github.com/mitchieui/mod-manager-cli/releases/download/v0.1.0-windows-preview.1/mmcli-windows-amd64.exe). Rename it to `mmcli.exe` and open PowerShell in that folder:

```powershell
.\mmcli.exe init
.\mmcli.exe tui
```

This detects your Valheim install, installs BepInEx, and creates a default profile. If detection fails, enter the folder containing `valheim.exe`.

To launch the game:

```powershell
.\mmcli.exe start
```

Use `.\mmcli.exe` in place of `mmcli` for the commands below. Windows profiles are stored in `%APPDATA%\mmcli`. Exit the game normally to let it save; Ctrl+C forcibly stops it on Windows. The dedicated-server agent remains Linux-only.

## Getting Started

```
mmcli init
```

This detects your Valheim install, installs BepInEx, and creates a default profile.

## Interactive TUI

```
mmcli tui
```

A terminal UI for browsing, toggling, installing, updating, and removing mods with keyboard shortcuts.

## Launching the Game

```
mmcli start
```

Launches Valheim with BepInEx loaded and streams logs to the terminal.

## Installing Mods

```
mmcli install RandyKnapp-EpicLoot
```

Dependencies are resolved and installed automatically.

Package names are looked up on Thunderstore first, then Hexium. To choose Hexium explicitly:

```
mmcli install hexium:Azumatt-AzuAutoStore
mmcli install https://valheim.hexium.gg/mods/Azumatt/AzuAutoStore
```

## Managing Mods

```
mmcli list                        # show installed mods
mmcli remove <mod>                # remove a mod and orphaned dependencies
```

## Profiles

Profiles let you maintain separate sets of mods (e.g. one for solo, one for a modded server).

```
mmcli profile create <name>
mmcli profile switch <name>
mmcli profile list
mmcli profile delete <name>
mmcli profile import <url|code>   # import from r2modman/Thunderstore profile code
mmcli profile open                # open profile folder in Finder or Explorer
mmcli profile export <name> <archive.zip>   # export installed mods and configs
mmcli profile restore <name> <archive.zip>  # restore into a new profile
```

## Moving a Profile to Windows

Close the game, then use this fork's Mac build to export a profile:

```
mmcli profile export default default.zip
```

Copy the archive to your Windows computer. After running `init`, restore it under a new name:

```powershell
.\mmcli.exe profile restore mac-default .\default.zip
.\mmcli.exe profile switch mac-default
.\mmcli.exe start
```

This preserves installed mod files, versions, sources, disabled states and configuration files. Existing profiles are never overwritten, and your Mac profile is unchanged.

Exports include `plugins`, `config`, `patchers` and `monomod`; mods with tracked files outside those folders cannot be exported. BepInEx is installed separately on Windows. Game saves, server connections and local modpack paths are not included. Reconnect servers and adjust any mod-specific paths after moving. Keep profile archives private, since mod configs may contain personal settings.
