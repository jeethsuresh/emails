package syncer

import "testing"

func TestFolderNameMatchKind(t *testing.T) {
	tests := []struct {
		folder, target, want string
	}{
		{"billing/subscriptions", "billing/subscriptions", "exact"},
		{"folders/billing/subscriptions", "billing/subscriptions", "suffix"},
		{"folders/billing/subscriptions", "subscriptions", "suffix"},
		{"billing/invoices", "subscriptions", ""},
		{"folders/billing/subscriptions", "billing", "partial"},
		{"inbox", "spam", ""},
	}
	for _, tc := range tests {
		if got := folderNameMatchKind(tc.folder, tc.target); got != tc.want {
			t.Fatalf("folderNameMatchKind(%q, %q) = %q, want %q", tc.folder, tc.target, got, tc.want)
		}
	}
}

func TestPrefixMailboxWithNamespace(t *testing.T) {
	tests := []struct {
		ns, name, want string
	}{
		{"Folders", "Billing/Subscriptions", "Folders/Billing/Subscriptions"},
		{"Folders", "Folders/Billing", "Folders/Billing"},
		{"Folders", "folders/Billing", "folders/Billing"},
		{"Folders", "travel", "Folders/travel"},
		{"", "Billing", "Billing"},
		{"Folders", "", ""},
	}
	for _, tc := range tests {
		if got := prefixMailboxWithNamespace(tc.ns, tc.name); got != tc.want {
			t.Fatalf("prefixMailboxWithNamespace(%q, %q) = %q, want %q", tc.ns, tc.name, got, tc.want)
		}
	}
}
