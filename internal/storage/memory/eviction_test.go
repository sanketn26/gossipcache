package memory

import "testing"

func TestLRUPolicySelectsLeastRecentlyUsedVictim(t *testing.T) {
	policy := newLRUPolicy()

	policy.OnAdd("a")
	policy.OnAdd("b")
	policy.OnAdd("c")
	policy.OnAccess("a")

	if victim := policy.SelectVictim(); victim != "b" {
		t.Fatalf("SelectVictim() = %q, want b", victim)
	}

	policy.OnRemove("b")
	if victim := policy.SelectVictim(); victim != "c" {
		t.Fatalf("SelectVictim() after removing b = %q, want c", victim)
	}
}

func TestLRUPolicyOnAddExistingMovesToFront(t *testing.T) {
	policy := newLRUPolicy()

	policy.OnAdd("a")
	policy.OnAdd("b")
	policy.OnAdd("a")

	if victim := policy.SelectVictim(); victim != "b" {
		t.Fatalf("SelectVictim() = %q, want b", victim)
	}
}
