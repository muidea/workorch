package main

import (
	"bytes"
	"context"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	repository "github.com/muidea/workorch/internal/initiators/workorchruntime"
	workorchd "github.com/muidea/workorch/internal/modules/application/workorchd"
	approval "github.com/muidea/workorch/internal/modules/blocks/approval"
	decision "github.com/muidea/workorch/internal/modules/blocks/decision"
	eventstream "github.com/muidea/workorch/internal/modules/blocks/eventstream"
	file "github.com/muidea/workorch/internal/modules/blocks/file"
	git "github.com/muidea/workorch/internal/modules/blocks/git"
	llm "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcp "github.com/muidea/workorch/internal/modules/blocks/mcp"
	mcpregistry "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	memory "github.com/muidea/workorch/internal/modules/blocks/memory"
	result "github.com/muidea/workorch/internal/modules/blocks/result"
	run "github.com/muidea/workorch/internal/modules/blocks/run"
	shell "github.com/muidea/workorch/internal/modules/blocks/shell"
	skill "github.com/muidea/workorch/internal/modules/blocks/skill"
	soul "github.com/muidea/workorch/internal/modules/blocks/soul"
	ssh "github.com/muidea/workorch/internal/modules/blocks/ssh"
	web "github.com/muidea/workorch/internal/modules/blocks/web"
	governance "github.com/muidea/workorch/internal/modules/kernel/governance"
	orchestrator "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	system "github.com/muidea/workorch/internal/modules/kernel/system"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

type stagedParticipant interface {
	ID() string
	BeginShutdown(context.Context)
	Quiesce(context.Context) *cd.Error
}

func TestProductionServerSignalWaitsForCheckedShutdown(t *testing.T) {
	addr := reserveLoopbackAddr(t)
	bin := buildWorkorchBinary(t)
	configDir := writeWorkorchTestConfig(t, addr, nil)
	var output bytes.Buffer
	cmd := exec.Command(bin, "--server", "-config-dir", configDir)
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	finished := false
	t.Cleanup(func() {
		if !finished {
			_ = cmd.Process.Kill()
			<-done
		}
	})
	waitForHealthy(t, "http://"+addr+"/healthz")
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatalf("checked shutdown exit=%v\n%s", err, output.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("production process did not finish checked shutdown within test budget")
	}
}

func TestProductionEntryParticipantsExposeFrameworkStagesAndRegister(t *testing.T) {
	participants := []struct {
		path  string
		owner stagedParticipant
	}{
		{"internal/initiators/workorchruntime", repository.New()},
		{"internal/modules/application/workorchd", workorchd.New()},
		{"internal/modules/blocks/approval", approval.New()},
		{"internal/modules/blocks/decision", decision.New()},
		{"internal/modules/blocks/eventstream", eventstream.New()},
		{"internal/modules/blocks/file", file.New()},
		{"internal/modules/blocks/git", git.New()},
		{"internal/modules/blocks/llm", llm.New()},
		{"internal/modules/blocks/mcp", mcp.New()},
		{"internal/modules/blocks/mcpregistry", mcpregistry.New()},
		{"internal/modules/blocks/memory", memory.New()},
		{"internal/modules/blocks/result", result.New()},
		{"internal/modules/blocks/run", run.New()},
		{"internal/modules/blocks/shell", shell.New()},
		{"internal/modules/blocks/skill", skill.New()},
		{"internal/modules/blocks/soul", soul.New()},
		{"internal/modules/blocks/ssh", ssh.New()},
		{"internal/modules/blocks/web", web.New()},
		{"internal/modules/kernel/governance", governance.New()},
		{"internal/modules/kernel/orchestrator", orchestrator.New()},
		{"internal/modules/kernel/system", system.New()},
		{"internal/modules/kernel/workspace", workspace.New()},
	}
	entry, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]bool{}
	for _, item := range entry.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if item.Name != nil && item.Name.Name == "_" {
			imports[path] = true
		}
	}
	if len(imports) != len(participants) {
		t.Fatalf("entry participants=%d want=%d", len(imports), len(participants))
	}
	modules, initiators := plugincommon.NewPluginMgr("module"), plugincommon.NewPluginMgr("initiator")
	ids := map[string]bool{}
	for idx, participant := range participants {
		if !imports["github.com/muidea/workorch/"+participant.path] {
			t.Fatalf("entry does not explicitly load %s", participant.path)
		}
		if ids[participant.owner.ID()] {
			t.Fatalf("duplicate owner %s", participant.owner.ID())
		}
		ids[participant.owner.ID()] = true
		manager := modules
		if idx == 0 {
			manager = initiators
		}
		if err := manager.Register(participant.owner); err != nil {
			t.Fatalf("%s: %v", participant.path, err)
		}
	}
}
