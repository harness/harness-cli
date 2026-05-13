package maven

import (
	"testing"
)

func TestParseDependencyTree(t *testing.T) {
	output := `[INFO] Scanning for projects...
[INFO]
[INFO] ------------------< io.harness.test:mvn-install-test >------------------
[INFO] Building mvn-install-test 1.0.0
[INFO]   from pom.xml
[INFO] --------------------------------[ jar ]---------------------------------
[INFO]
[INFO] --- dependency:3.7.0:tree (default-cli) @ mvn-install-test ---
[INFO] io.harness.test:mvn-install-test:jar:1.0.0
[INFO] +- com.google.guava:guava:jar:31.1-jre:compile
[INFO] |  +- com.google.guava:failureaccess:jar:1.0.1:compile
[INFO] |  +- com.google.guava:listenablefuture:jar:9999.0-empty-to-avoid-conflict-with-guava:compile
[INFO] |  +- com.google.code.findbugs:jsr305:jar:3.0.2:compile
[INFO] |  +- org.checkerframework:checker-qual:jar:3.12.0:compile
[INFO] |  +- com.google.errorprone:error_prone_annotations:jar:2.11.0:compile
[INFO] |  \- com.google.j2objc:j2objc-annotations:jar:1.3:compile
[INFO] \- org.apache.commons:commons-lang3:jar:3.12.0:compile
[INFO] ------------------------------------------------------------------------
[INFO] BUILD SUCCESS
[INFO] ------------------------------------------------------------------------`

	deps := parseDependencyTree(output)

	if len(deps) != 8 {
		t.Fatalf("expected 8 dependencies, got %d", len(deps))
	}

	// Check first dep (direct)
	if deps[0].Name != "com.google.guava:guava" {
		t.Errorf("expected first dep to be com.google.guava:guava, got %s", deps[0].Name)
	}
	if deps[0].Version != "31.1-jre" {
		t.Errorf("expected version 31.1-jre, got %s", deps[0].Version)
	}

	// Check transitive dep
	found := false
	for _, dep := range deps {
		if dep.Name == "com.google.guava:failureaccess" {
			found = true
			if dep.Version != "1.0.1" {
				t.Errorf("expected failureaccess version 1.0.1, got %s", dep.Version)
			}
			if dep.Parent == "" {
				t.Error("expected failureaccess to have a parent")
			}
			break
		}
	}
	if !found {
		t.Error("transitive dep com.google.guava:failureaccess not found")
	}

	// Check last dep (direct)
	lastDep := deps[len(deps)-1]
	if lastDep.Name != "org.apache.commons:commons-lang3" {
		t.Errorf("expected last dep to be commons-lang3, got %s", lastDep.Name)
	}

	// All deps should have source "mvn-dependency-tree"
	for _, dep := range deps {
		if dep.Source != "mvn-dependency-tree" {
			t.Errorf("expected source mvn-dependency-tree, got %s for %s", dep.Source, dep.Name)
		}
	}
}

func TestDetectFirewallError(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"403 Forbidden", "HTTP/1.1 403 Forbidden", true},
		{"status code 403", "Received status code: 403 from server", true},
		{"Return code 403", "Return code is: 403", true},
		{"no error", "BUILD SUCCESS", false},
		{"404 not 403", "HTTP/1.1 404 Not Found", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.DetectFirewallError(tt.stderr)
			if got != tt.want {
				t.Errorf("DetectFirewallError(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}
