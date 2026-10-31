<p align="center">
  <img src="assets/icons/icon.png" width="96" alt="sshgo">
</p>

# sshgo

[English](README.md)

原生 UI 的 SSH 客户端，支持 macOS、Windows 和 Linux。主机列表在侧栏，终端在旁边的卡片里。

## 功能

- 密码、私钥或 SSH Agent 登录；保存的密码与私钥口令存放在系统密钥库。
- 主机与分组；主机密钥按 `known_hosts` 校验。
- 终端支持 8 套淡彩主题（浅色/深色），可调整字体、字号与光标。
- 可选托盘：关闭到托盘、开机启动、启动时连接上次的主机。
- 检查更新；界面支持简体中文和英文。

## 安装

从 [Releases](https://github.com/rzh0504/sshgo/releases) 下载 macOS、Windows 或 Linux 安装包。

## 构建

需要 Go 1.27.3 或更高版本。

```sh
go tool mygo build
```

`go tool mygo dev` 用于开发调试。

## 开源协议

MIT，见 [LICENSE](LICENSE)。
