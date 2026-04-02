package guard

import "testing"

func TestCheckWorktreeCd_BlocksDirectCd(t *testing.T) {
	cases := []struct {
		name    string
		command string
	}{
		{"bare cd", "cd .claude/worktrees/dev-PRA-36tu"},
		{"cd with absolute path", "cd /Users/ramirosalas/workspace/praktical/.claude/worktrees/dev-PRA-36tu"},
		{"pushd", "pushd .claude/worktrees/dev-PRA-36tu"},
		{"chained after &&", "echo hello && cd .claude/worktrees/dev-PRA-36tu"},
		{"chained after ;", "echo hello; cd .claude/worktrees/dev-PRA-36tu"},
		{"cd with trailing command", "cd .claude/worktrees/dev-X && ls"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := CheckWorktreeCd(tc.command)
			if r.Allowed {
				t.Errorf("expected BLOCKED for %q", tc.command)
			}
		})
	}
}

func TestCheckWorktreeCd_AllowsLegitimateCommands(t *testing.T) {
	cases := []struct {
		name    string
		command string
	}{
		{"empty", ""},
		{"no worktree ref", "cd /Users/ramirosalas/workspace/praktical"},
		{"git worktree add", "git worktree add .claude/worktrees/dev-PRA-36tu story/PRA-36tu"},
		{"pvg worktree remove", "pvg worktree remove .claude/worktrees/dev-PRA-36tu"},
		{"ls worktree dir", "ls .claude/worktrees/dev-PRA-36tu"},
		{"git -C worktree", "git -C .claude/worktrees/dev-PRA-36tu log --oneline"},
		{"cat file in worktree", "cat .claude/worktrees/dev-PRA-36tu/mix.exs"},
		{"grep in worktree", "grep -r 'pattern' .claude/worktrees/dev-PRA-36tu/"},
		{"cd to project root with worktree remove", "cd /project/root && pvg worktree remove .claude/worktrees/dev-X"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := CheckWorktreeCd(tc.command)
			if !r.Allowed {
				t.Errorf("expected ALLOWED for %q, got blocked: %s", tc.command, r.Reason)
			}
		})
	}
}
