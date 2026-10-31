<p align="center">
  <img src="assets/icons/icon.png" width="96" alt="sshgo">
</p>

# sshgo

[简体中文](README.zh.md)

An SSH client with native UI for macOS, Windows and Linux. Hosts in a sidebar, their shells in a card beside it.

## Features

- Password, private-key or SSH-agent sign-in; saved passwords and key passphrases go to the system keychain.
- Hosts and groups; host keys are checked against `known_hosts`.
- Terminals with eight pastel themes, light and dark, and a configurable font, size and cursor.
- Optional tray: close to the tray, open at login, connect the last host on launch.
- Update checks, and an English or Simplified Chinese interface.

## Install

Download the macOS, Windows or Linux package from [Releases](https://github.com/rzh0504/sshgo/releases).

## Build

Go 1.27.3 or later.

```sh
go tool mygo build
```

`go tool mygo dev` runs a development build.

## License

MIT. See [LICENSE](LICENSE).
