package agent

import (
	"path"
	"strings"
)

// Risk is how much a command may change: the app asks about everything,
// but knows a command it would not let run unattended.
type Risk int

const (
	RiskReadOnly Risk = iota
	RiskUnknown
	RiskCaution
	RiskDestructive
)

// String names the risk for tests and logs.
func (r Risk) String() string {
	switch r {
	case RiskReadOnly:
		return "read-only"
	case RiskCaution:
		return "caution"
	case RiskDestructive:
		return "destructive"
	}
	return "unknown"
}

// Classify judges a shell command by its shape. It is an allowlist: only
// a command whose every part it recognises as read-only is RiskReadOnly;
// anything it does not recognise is RiskUnknown, which is still reviewed.
func Classify(command string) Risk {
	segments := splitSegments(command)
	if len(segments) == 0 {
		return RiskUnknown
	}
	risk := RiskReadOnly
	// A command substitution runs whatever it holds, which the shape of
	// the command around it cannot judge: never read-only.
	if strings.Contains(command, "$(") || strings.Contains(command, "`") ||
		strings.Contains(command, "<(") || strings.Contains(command, ">(") {
		risk = RiskUnknown
	}
	for _, segment := range segments {
		words := splitWords(segment)
		if len(words) == 0 {
			continue
		}
		risk = worseRisk(risk, segmentRisk(words))
	}
	return risk
}

func worseRisk(a, b Risk) Risk {
	if a == RiskDestructive || b == RiskDestructive {
		return RiskDestructive
	}
	if a == RiskCaution || b == RiskCaution {
		return RiskCaution
	}
	if a == RiskUnknown || b == RiskUnknown {
		return RiskUnknown
	}
	return RiskReadOnly
}

func segmentRisk(words []string) Risk {
	redirect := RiskReadOnly
	if hasWriteRedirect(words) {
		redirect = RiskCaution
	}
	elevated := false
	assigned := false
	for len(words) > 0 {
		if isAssignment(words[0]) {
			assigned = true
			words = words[1:]
			continue
		}
		switch words[0] {
		case "sudo", "doas":
			elevated = true
			words = skipWrappers(words[1:])
		case "env":
			words = words[1:]
			words = skipFlags(words)
		case "nice", "nohup", "time", "command", "builtin", "exec":
			words = skipFlags(words[1:])
		default:
			goto analyze
		}
	}
analyze:
	if len(words) == 0 {
		return RiskUnknown
	}
	// A command named by path may not be the program its name suggests:
	// ./ls is whatever is in that directory.
	if strings.ContainsAny(words[0], `/\`) {
		return worseRisk(RiskUnknown, redirect)
	}
	name := path.Base(words[0])
	risk := commandRisk(name, words[1:])
	if (elevated || assigned) && risk == RiskReadOnly {
		risk = RiskCaution
	}
	return worseRisk(risk, redirect)
}

// skipWrappers drops the options a wrapper such as sudo takes before the
// command it runs.
func skipWrappers(words []string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		option := words[0]
		words = words[1:]
		switch option {
		case "-u", "-g", "-p", "-C", "-h", "-r", "-t", "--user", "--group":
			if len(words) > 0 {
				words = words[1:]
			}
		}
	}
	for len(words) > 0 && isAssignment(words[0]) {
		words = words[1:]
	}
	return words
}

func skipFlags(words []string) []string {
	for len(words) > 0 && strings.HasPrefix(words[0], "-") {
		words = words[1:]
	}
	return words
}

func isAssignment(word string) bool {
	i := strings.IndexByte(word, '=')
	if i <= 0 {
		return false
	}
	for _, r := range word[:i] {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func commandRisk(name string, args []string) Risk {
	switch name {
	case "ls", "pwd", "cat", "head", "tail", "grep", "egrep", "fgrep", "zgrep",
		"wc", "stat", "file", "du", "df", "free", "uptime", "uname", "whoami",
		"id", "groups", "echo", "printf", "ps", "pgrep", "ss", "netstat",
		"getent", "dig", "nslookup", "host", "ping", "traceroute", "mtr",
		"lscpu", "lsblk", "blkid", "lsof", "which", "type", "readlink", "realpath",
		"basename", "dirname", "md5sum", "sha1sum", "sha256sum", "sha512sum",
		"sort", "uniq", "cut", "tr", "column", "rev", "seq", "yes",
		"env", "printenv", "ulimit", "umask", "sensors", "vcgencmd",
		"nproc", "arch", "tty", "w", "who", "last", "lastb", "pidof",
		"lsusb", "lspci", "dmidecode", "tree", "jq", "base64", "xxd", "od",
		"cmp", "diff", "comm", "paste", "join", "split", "csplit",
		"vmstat", "iostat", "mpstat", "sar", "dmesg", "ipcs", "ipcrm",
		"findmnt", "getenforce", "sestatus", "aa-status":
		return simpleRisk(name, args)
	case "date":
		if hasArg(args, "-s", "--set") {
			return RiskCaution
		}
		return RiskReadOnly
	case "hostname":
		for _, a := range args {
			if a == "-F" || a == "--file" || a == "-b" || a == "--boot" {
				return RiskCaution
			}
		}
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			return RiskCaution
		}
		return RiskReadOnly
	case "ip":
		for _, a := range args {
			switch a {
			case "add", "del", "delete", "set", "change", "replace", "flush", "up", "down", "append", "prepend":
				return RiskCaution
			}
		}
		return RiskReadOnly
	case "route":
		for _, a := range args {
			switch a {
			case "add", "del", "delete", "flush", "change":
				return RiskCaution
			}
		}
		return RiskReadOnly
	case "journalctl":
		for _, a := range args {
			switch a {
			case "--vacuum-size", "--vacuum-time", "--vacuum-files", "--rotate", "--flush", "--sync", "--relinquish-var", "--setup-keys":
				return RiskCaution
			}
		}
		return RiskReadOnly
	case "systemctl":
		return systemctlRisk(args)
	case "service":
		if len(args) == 0 {
			return RiskUnknown
		}
		action := args[0]
		if len(args) > 1 {
			action = args[1]
		}
		switch action {
		case "status":
			return RiskReadOnly
		case "start", "reload":
			return RiskCaution
		case "stop", "restart", "force-reload":
			return RiskDestructive
		}
		return RiskUnknown
	case "sysctl":
		if hasArg(args, "-w", "--write") {
			return RiskCaution
		}
		return RiskReadOnly
	case "hostnamectl", "timedatectl", "localectl":
		if anyString(args, "set-") {
			return RiskCaution
		}
		return RiskReadOnly
	case "loginctl":
		for _, a := range args {
			switch a {
			case "terminate-session", "terminate-user", "terminate-seat", "kill-session", "kill-user", "kill-seat":
				return RiskDestructive
			}
		}
		return RiskReadOnly
	case "nmcli":
		if len(args) == 0 {
			return RiskReadOnly
		}
		switch args[0] {
		case "up", "down", "connect", "disconnect", "add", "modify", "delete", "reload":
			return RiskCaution
		}
		return RiskReadOnly
	case "nft":
		if len(args) == 0 {
			return RiskUnknown
		}
		switch args[0] {
		case "list":
			return RiskReadOnly
		case "flush":
			return RiskDestructive
		}
		return RiskCaution
	case "ufw":
		if len(args) == 0 {
			return RiskReadOnly
		}
		switch args[0] {
		case "status":
			return RiskReadOnly
		case "disable", "reset":
			return RiskDestructive
		}
		return RiskCaution
	case "iptables", "ip6tables":
		for _, a := range args {
			switch a {
			case "-L", "-S", "--list", "--list-rules":
				return RiskReadOnly
			case "-F", "-X", "-P", "--flush", "--delete-chain", "--policy":
				return RiskDestructive
			case "-A", "-I", "-D", "-R", "-N", "-E", "--append", "--insert", "--delete", "--replace", "--new-chain", "--rename-chain":
				return RiskCaution
			}
		}
		return RiskUnknown
	case "firewall-cmd":
		for _, a := range args {
			if strings.HasPrefix(a, "--list") || strings.HasPrefix(a, "--get") || strings.HasPrefix(a, "--query") || a == "--state" {
				return RiskReadOnly
			}
		}
		return RiskCaution
	case "mount":
		if len(args) == 0 || hasArg(args, "-l", "--list") {
			return RiskReadOnly
		}
		return RiskCaution
	case "umount", "swapon":
		return RiskCaution
	case "swapoff":
		return RiskDestructive
	case "crontab":
		if hasArg(args, "-l", "--list") {
			return RiskReadOnly
		}
		if hasArg(args, "-r", "--remove") {
			return RiskDestructive
		}
		return RiskCaution
	case "apt", "apt-get", "yum", "dnf", "zypper":
		sub := firstWord(args)
		switch sub {
		case "list", "search", "show", "policy", "info", "check", "provides", "whatprovides", "repoquery":
			return RiskReadOnly
		case "install", "reinstall", "upgrade", "update", "dist-upgrade", "full-upgrade", "source", "clean", "autoclean", "download":
			return RiskCaution
		case "remove", "purge", "autoremove", "erase", "groupremove":
			return RiskDestructive
		}
		return RiskUnknown
	case "apk":
		switch firstWord(args) {
		case "info", "list", "search", "policy", "version", "stats":
			return RiskReadOnly
		case "add", "update", "upgrade", "fix", "cache":
			return RiskCaution
		case "del", "delete", "remove":
			return RiskDestructive
		}
		return RiskUnknown
	case "pacman":
		if hasArgPrefix(args, "-Q", "--query") {
			return RiskReadOnly
		}
		if hasArgPrefix(args, "-R", "--remove") {
			return RiskDestructive
		}
		if hasArgPrefix(args, "-S", "--sync") || hasArgPrefix(args, "-U", "--upgrade") {
			return RiskCaution
		}
		return RiskUnknown
	case "dpkg":
		if hasArgPrefix(args, "-l", "--list") || hasArgPrefix(args, "-L") || hasArgPrefix(args, "-s", "--status") {
			return RiskReadOnly
		}
		if hasArgPrefix(args, "-r", "--remove") || hasArgPrefix(args, "-P", "--purge") {
			return RiskDestructive
		}
		if hasArgPrefix(args, "-i", "--install") {
			return RiskCaution
		}
		return RiskUnknown
	case "rpm":
		if hasArgPrefix(args, "-q", "--query") {
			return RiskReadOnly
		}
		if hasArgPrefix(args, "-e", "--erase") {
			return RiskDestructive
		}
		if hasArgPrefix(args, "-i", "--install") || hasArgPrefix(args, "-U", "--upgrade") {
			return RiskCaution
		}
		return RiskUnknown
	case "snap":
		switch firstWord(args) {
		case "list", "info", "find", "services", "changes", "version":
			return RiskReadOnly
		case "install", "refresh", "revert", "switch", "enable", "disable", "start", "stop", "restart":
			return RiskCaution
		case "remove":
			return RiskDestructive
		}
		return RiskUnknown
	case "flatpak":
		switch firstWord(args) {
		case "list", "info", "search", "remote-ls", "remotes":
			return RiskReadOnly
		case "install", "update", "uninstall", "override", "make-current":
			return RiskCaution
		}
		return RiskUnknown
	case "docker", "podman":
		return containerRisk(args)
	case "kubectl":
		switch firstWord(args) {
		case "get", "describe", "logs", "top", "explain", "api-resources", "api-versions", "version", "events", "cluster-info":
			return RiskReadOnly
		case "delete", "drain":
			return RiskDestructive
		case "apply", "create", "replace", "edit", "scale", "rollout", "exec", "port-forward", "patch", "set", "annotate", "label", "cordon", "uncordon", "taint":
			return RiskCaution
		}
		return RiskUnknown
	case "git":
		return gitRisk(args)
	case "sed":
		if hasArgPrefix(args, "-i", "--in-place") || hasArg(args, "-i") {
			return RiskCaution
		}
		// A sed script can run another command (the e command) and write
		// a file (w); the shape alone does not rule that out.
		return RiskUnknown
	case "find":
		for _, a := range args {
			switch a {
			case "-delete":
				return RiskDestructive
			case "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls":
				return RiskCaution
			}
		}
		return RiskReadOnly
	case "tar":
		if hasArgPrefix(args, "-x", "--extract") || hasArgPrefix(args, "-c", "--create") || hasArgPrefix(args, "-r", "--append") || hasArgPrefix(args, "-u", "--update") {
			return RiskCaution
		}
		if hasArgPrefix(args, "-t", "--list") || hasArgPrefix(args, "-d", "--diff") {
			return RiskReadOnly
		}
		return RiskUnknown
	case "zip":
		return RiskCaution
	case "unzip":
		if hasArg(args, "-l", "-t", "-v") {
			return RiskReadOnly
		}
		return RiskCaution
	case "chmod", "chown", "chgrp", "chattr", "setfacl", "touch", "mkdir", "ln", "tee", "fallocate":
		return RiskCaution
	case "mv", "cp", "install", "rsync", "scp":
		return RiskCaution
	case "rm", "rmdir", "shred", "truncate", "dd", "mkfs", "wipefs", "fdisk", "parted", "sfdisk", "userdel", "groupdel":
		if name == "fdisk" || name == "parted" {
			if hasArg(args, "-l", "--list") {
				return RiskReadOnly
			}
		}
		return RiskDestructive
	case "mkfs.ext4", "mkfs.xfs", "mkfs.btrfs", "mkfs.vfat", "mkfs.fat", "mkfs.zfs":
		return RiskDestructive
	case "mdadm":
		for _, a := range args {
			switch a {
			case "--detail", "--examine", "--query", "-D", "-E":
				return RiskReadOnly
			}
		}
		return RiskCaution
	case "smartctl":
		for _, a := range args {
			switch a {
			case "-a", "-x", "-H", "-i", "-A", "-c", "-l", "--all", "--xall", "--health", "--info", "--attributes":
				return RiskReadOnly
			}
		}
		if hasArgPrefix(args, "-t", "--test") {
			return RiskCaution
		}
		return RiskUnknown
	case "kill", "pkill", "killall":
		return RiskDestructive
	case "reboot", "shutdown", "poweroff", "halt", "init", "telinit":
		return RiskDestructive
	case "useradd", "usermod", "groupadd", "groupmod", "passwd", "chpasswd", "visudo", "setenforce":
		return RiskCaution
	case "xargs", "awk", "gawk", "mawk", "perl", "python", "python3", "ruby", "node", "sh", "bash", "zsh", "fish", "eval", "source":
		return RiskUnknown
	case "curl", "wget", "nc", "ncat", "netcat", "socat", "ssh", "telnet", "ftp", "sftp", "sqlite3", "mysql", "psql", "redis-cli", "mongo", "mongosh":
		return RiskUnknown
	case "watch", "screen", "tmux", "less", "more", "vi", "vim", "nano", "emacs", "top", "htop", "atop":
		return RiskUnknown
	}
	return RiskUnknown
}

// simpleRisk is the verdict on a listing or reading command, with the
// few flags that make one change something.
func simpleRisk(name string, args []string) Risk {
	if name == "docker" {
		return containerRisk(args)
	}
	switch name {
	case "findmnt", "getenforce", "sestatus", "aa-status":
		return RiskReadOnly
	}
	for _, a := range args {
		switch a {
		case "-delete", "-exec", "-execdir", "-ok":
			if name == "find" {
				return RiskDestructive
			}
		}
	}
	return RiskReadOnly
}

func systemctlRisk(args []string) Risk {
	if len(args) == 0 {
		return RiskReadOnly // lists units
	}
	switch args[0] {
	case "status", "is-active", "is-enabled", "is-failed", "is-system-running",
		"list-units", "list-unit-files", "list-timers", "list-sockets", "list-jobs",
		"show", "cat", "help", "get-default":
		return RiskReadOnly
	case "start", "reload", "reload-or-restart", "try-restart", "enable", "unmask", "daemon-reload", "daemon-reexec", "set-default":
		return RiskCaution
	case "stop", "restart", "disable", "mask", "kill", "isolate", "poweroff", "reboot", "halt", "suspend", "hibernate", "emergency", "rescue":
		return RiskDestructive
	}
	return RiskUnknown
}

func containerRisk(args []string) Risk {
	if len(args) == 0 {
		return RiskReadOnly
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "ps", "images", "logs", "inspect", "stats", "version", "info", "port", "top", "diff", "history", "events":
		return RiskReadOnly
	case "container", "image", "volume", "network", "system", "compose":
		if len(rest) == 0 {
			return RiskReadOnly
		}
		switch sub + " " + rest[0] {
		case "container ls", "container list", "container inspect", "container logs", "container stats", "container top", "container port", "container diff",
			"image ls", "image list", "image inspect", "image history",
			"volume ls", "volume list", "volume inspect",
			"network ls", "network list", "network inspect",
			"system df", "system info", "system events",
			"compose ps", "compose logs", "compose config":
			return RiskReadOnly
		}
		switch rest[0] {
		case "rm", "remove", "prune", "stop", "kill":
			return RiskDestructive
		}
		return RiskCaution
	case "rm", "rmi", "stop", "kill", "prune":
		return RiskDestructive
	case "run", "create", "start", "restart", "exec", "pull", "push", "build", "commit", "tag", "login", "logout", "save", "load":
		return RiskCaution
	}
	return RiskUnknown
}

func gitRisk(args []string) Risk {
	if len(args) == 0 {
		return RiskReadOnly
	}
	switch args[0] {
	case "status", "log", "diff", "show", "rev-parse", "ls-files", "ls-tree",
		"blame", "annotate", "describe", "shortlog", "grep", "whatchanged",
		"count-objects", "fsck", "show-ref", "for-each-ref", "cat-file":
		return RiskReadOnly
	case "branch", "tag":
		for _, a := range args[1:] {
			switch a {
			case "-d", "-D", "--delete", "-m", "-M", "--move", "-a", "--annotate", "-f", "--force":
				return RiskCaution
			}
		}
		return RiskReadOnly
	case "remote":
		if len(args) == 1 {
			return RiskReadOnly
		}
		switch args[1] {
		case "-v", "--verbose", "show", "get-url":
			return RiskReadOnly
		}
		return RiskCaution
	case "config":
		if hasArg(args, "--get", "--get-all", "--list", "-l") {
			return RiskReadOnly
		}
		return RiskCaution
	case "stash":
		if len(args) > 1 && (args[1] == "list" || args[1] == "show") {
			return RiskReadOnly
		}
		return RiskCaution
	case "reset":
		if hasArg(args, "--hard") {
			return RiskDestructive
		}
		return RiskCaution
	case "clean":
		if hasArgPrefix(args, "-f", "--force") {
			return RiskDestructive
		}
		return RiskCaution
	case "add", "rm", "mv", "checkout", "switch", "restore", "pull", "fetch",
		"merge", "rebase", "cherry-pick", "revert", "commit", "apply", "am", "push":
		return RiskCaution
	}
	return RiskUnknown
}

func hasWriteRedirect(words []string) bool {
	for i, w := range words {
		if strings.Contains(w, ">&") {
			continue
		}
		pos := strings.IndexByte(w, '>')
		if pos < 0 {
			continue
		}
		target := strings.TrimLeft(w[pos:], ">")
		if target == "" && i+1 < len(words) {
			target = words[i+1]
		}
		if target != "" && target != "/dev/null" && !strings.HasPrefix(target, "&") {
			return true
		}
	}
	return false
}

func hasArg(args []string, names ...string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n {
				return true
			}
		}
	}
	return false
}

func hasArgPrefix(args []string, prefixes ...string) bool {
	for _, a := range args {
		for _, p := range prefixes {
			if strings.HasPrefix(a, p) {
				return true
			}
		}
	}
	return false
}

func anyString(args []string, substr string) bool {
	for _, a := range args {
		if strings.Contains(a, substr) {
			return true
		}
	}
	return false
}

func firstWord(args []string) string {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

// splitSegments splits a command at the operators that run one part after
// another. Quotes keep their contents together.
func splitSegments(command string) []string {
	var (
		out     []string
		b       strings.Builder
		quote   byte
		escaped bool
	)
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			out = append(out, s)
		}
		b.Reset()
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			b.WriteByte(c)
			continue
		}
		if quote != 0 {
			b.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			b.WriteByte(c)
		case ';', '\n':
			flush()
		case '&':
			// & is a separator unless it is part of a redirection, as in
			// 2>&1 or &>file.
			if i > 0 && command[i-1] == '>' || i+1 < len(command) && command[i+1] == '>' {
				b.WriteByte(c)
				continue
			}
			flush()
			if i+1 < len(command) && command[i+1] == '&' {
				i++
			}
		case '|':
			flush()
			if i+1 < len(command) && command[i+1] == '|' {
				i++
			}
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return out
}

func splitWords(s string) []string {
	var (
		words   []string
		b       strings.Builder
		quote   byte
		escaped bool
	)
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(c)
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case ' ', '\t', '\r':
			flush()
		default:
			b.WriteByte(c)
		}
	}
	flush()
	return words
}
