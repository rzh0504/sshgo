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
	safe := ShellArgs{SafeToRun: true}
	if !(Policy{AutoRunSafe: true}).Allows(safe, RiskReadOnly, 0) {
		t.Fatal("a safe read-only command should run unattended when allowed")
	}
	if (Policy{}).Allows(safe, RiskReadOnly, 0) {
		t.Fatal("the policy ran a command while off")
	}
	if (Policy{AutoRunSafe: true}).Allows(safe, RiskCaution, 0) {
		t.Fatal("a caution command ran unattended")
	}
	if (Policy{AutoRunSafe: true}).Allows(ShellArgs{}, RiskReadOnly, 0) {
		t.Fatal("a command the model did not mark safe ran unattended")
	}
	if (Policy{AutoRunSafe: true}).Allows(safe, RiskReadOnly, MaxAutoRuns) {
		t.Fatal("the auto-run limit was not kept")
	}
}
