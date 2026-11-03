package agent

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		command string
		want    Risk
	}{
		{"ls -la /var/log", RiskReadOnly},
		{"cat /etc/os-release | grep ^NAME", RiskReadOnly},
		{"df -h && free -m", RiskReadOnly},
		{"systemctl status nginx", RiskReadOnly},
		{"journalctl -u nginx --no-pager -n 100", RiskReadOnly},
		{`find /var/log -name "*.log"`, RiskReadOnly},
		{"git status --short", RiskReadOnly},
		{"docker ps -a", RiskReadOnly},
		{"docker compose ps", RiskReadOnly},
		{"ip addr show", RiskReadOnly},
		{"echo hi > /dev/null", RiskReadOnly},
		{"cat /etc/passwd 2>/dev/null", RiskReadOnly},
		{"echo hi 2>&1", RiskReadOnly},
		{"apt list --installed", RiskReadOnly},

		{"rm -rf /tmp/x", RiskDestructive},
		{"sudo rm -rf /", RiskDestructive},
		{"systemctl restart nginx", RiskDestructive},
		{"service nginx stop", RiskDestructive},
		{"reboot", RiskDestructive},
		{"find . -delete", RiskDestructive},
		{"docker rm web", RiskDestructive},
		{"docker system prune -af", RiskDestructive},
		{"kubectl delete pod web", RiskDestructive},
		{"git reset --hard HEAD~1", RiskDestructive},
		{"git clean -fd", RiskDestructive},
		{"apt-get remove nginx", RiskDestructive},
		{"fdisk /dev/sda", RiskDestructive},
		{"fdisk -l", RiskReadOnly},

		{"echo hi > /tmp/x", RiskCaution},
		{"sudo ls", RiskCaution},
		{"mv a b", RiskCaution},
		{"chmod 777 /tmp/x", RiskCaution},
		{"sed -i 's/a/b/' /etc/hosts", RiskCaution},
		{"apt-get update", RiskCaution},
		{"ip link set eth0 down", RiskCaution},
		{"git checkout main", RiskCaution},

		{"curl https://example.com", RiskUnknown},
		{"unknowncmd --flag", RiskUnknown},
		{"", RiskUnknown},
		{"sed -n '1,10p' /etc/hosts", RiskUnknown},
		{"echo $(rm -rf /)", RiskUnknown},
		{"echo `id`", RiskUnknown},
		{"X=1 ls", RiskCaution},
		{"./ls", RiskUnknown},
		{"/bin/ls", RiskUnknown},
	}
	for _, tt := range tests {
		if got := Classify(tt.command); got != tt.want {
			t.Errorf("Classify(%q) = %s, want %s", tt.command, got, tt.want)
		}
	}
}

func TestPolicyAllows(t *testing.T) {
	tool := Prepared{Auto: AutoReadTool, Risk: RiskReadOnly}
	command := Prepared{Auto: AutoReadCommand, ModelSafe: true, Risk: RiskReadOnly}
	if !(Policy{ReadTools: true}).Allows(tool) {
		t.Fatal("a read-only tool should run unattended when allowed")
	}
	if !(Policy{ReadCommands: true}).Allows(command) {
		t.Fatal("a safe read-only command should run unattended when allowed")
	}
	if (Policy{ReadCommands: true}).Allows(tool) {
		t.Fatal("a read-only tool used the command setting")
	}
	if (Policy{ReadTools: true}).Allows(command) {
		t.Fatal("a read-only command used the tool setting")
	}
	if (Policy{}).Allows(tool) {
		t.Fatal("the policy ran an action while off")
	}
	caution := tool
	caution.Risk = RiskCaution
	if (Policy{ReadTools: true}).Allows(caution) {
		t.Fatal("a caution action ran unattended")
	}
	unsafe := command
	unsafe.ModelSafe = false
	if (Policy{ReadCommands: true}).Allows(unsafe) {
		t.Fatal("a command the model did not mark safe ran unattended")
	}
	write := tool
	write.Auto = AutoNone
	if (Policy{ReadTools: true}).Allows(write) {
		t.Fatal("an action that always asks ran unattended")
	}
}
