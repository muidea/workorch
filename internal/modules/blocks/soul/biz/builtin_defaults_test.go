package soul

import (
	"context"
	_ "embed"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepo "github.com/muidea/workorch/internal/pkg/repository/file"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchdefaults"
)

// Frozen Git-reviewed inputs must not be reconstructed from the latest Soul:
// otherwise every guidance edit silently changes the historical test baseline.
//
//go:embed testdata/software-dev-execution-fe73385d.md
var historicalExecutionSoulMD string

//go:embed testdata/software-dev-execution-0be3a5f3.md
var previousExecutionSoulMD string

//go:embed testdata/software-dev-validation-0be3a5f3.md
var previousValidationSoulMD string

//go:embed testdata/software-dev-execution-0317a3f6.md
var previousInputExecutionSoulMD string

//go:embed testdata/software-dev-validation-0317a3f6.md
var previousInputValidationSoulMD string

func TestPreviousValidationInputSoulsReviewedFingerprints(t *testing.T) {
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, raw := range defaults {
		var md string
		switch raw.ID {
		case "soul-software-dev-execution":
			md = previousInputExecutionSoulMD
		case "soul-software-dev-validation":
			md = previousInputValidationSoulMD
		default:
			continue
		}
		cmd := soulDefaultCreateCommand(raw)
		cmd.SoulMD = md
		value, err := normalizedSoulFromCommand(cmd)
		if err != nil {
			t.Fatal(err)
		}
		hash := soulmodel.StableSoulContentHash(value)
		if got := workorchdefaults.KnownBuiltinContentRevision("soul", raw.ID, hash); got != "0317a3f6" {
			t.Fatalf("previous Soul not reviewed: %s %s %s", raw.ID, hash, got)
		}
		checked++
	}
	if checked != 2 {
		t.Fatalf("checked previous Souls = %d", checked)
	}
}

func historicalExecutionSoul(t *testing.T) soulevents.CreateSoulCommand {
	t.Helper()
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range defaults {
		if raw.ID != "soul-software-dev-execution" {
			continue
		}
		cmd := soulDefaultCreateCommand(raw)
		cmd.SoulMD = historicalExecutionSoulMD
		value, err := normalizedSoulFromCommand(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if hash := soulmodel.StableSoulContentHash(value); hash != "soul-content-sha256:902992d62f52413de538e6a8d9624cce06537c8cbcfe0956482b09f80bc4f225" {
			t.Fatalf("historical fixture changed: %s", hash)
		}
		return cmd
	}
	t.Fatal("missing builtin execution Soul")
	return soulevents.CreateSoulCommand{}
}

func TestPreviousSoftwareDevSoulReviewedFingerprints(t *testing.T) {
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range defaults {
		md := ""
		switch raw.ID {
		case "soul-software-dev-execution":
			md = previousExecutionSoulMD
		case "soul-software-dev-validation":
			md = previousValidationSoulMD
		default:
			continue
		}
		cmd := soulDefaultCreateCommand(raw)
		cmd.SoulMD = md
		value, err := normalizedSoulFromCommand(cmd)
		if err != nil {
			t.Fatal(err)
		}
		hash := soulmodel.StableSoulContentHash(value)
		if workorchdefaults.KnownBuiltinContentRevision("soul", raw.ID, hash) == "" {
			t.Fatalf("previous %s is not reviewed: %s", raw.ID, hash)
		}
		for _, backend := range []string{"memory", "file"} {
			t.Run(raw.ID+"/"+backend, func(t *testing.T) {
				ctx := context.Background()
				root := filepath.Join(t.TempDir(), "souls")
				var repo repository.SoulRepository = memoryrepo.NewSoulRepository()
				if backend == "file" {
					repo = filerepo.NewSoulRepository(root)
				}
				old, err := newService(repo).createSoul(ctx, cmd, soulmodel.SoulSourceBuiltinSeed)
				if err != nil {
					t.Fatal(err)
				}
				for attempt := 0; attempt < 2; attempt++ {
					if backend == "file" {
						repo = filerepo.NewSoulRepository(root)
					}
					s := newService(repo)
					if err := s.ensureCurrentBuiltinSoulDefaults(ctx); err != nil {
						t.Fatal(err)
					}
					current, err := s.getSoul(ctx, old.ID)
					if err != nil || current.Revision != old.Revision+1 || current.ContentHash == old.ContentHash {
						t.Fatalf("previous seed did not load/replay latest content: %+v %v", current, err)
					}
				}
			})
		}
	}
}

func TestSoftwareDevExecutionGuidanceIsFactBound(t *testing.T) {
	defaults, err := workorchdefaults.BuiltinSoulDefaults()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range defaults {
		if raw.ID != "soul-software-dev-execution" {
			continue
		}
		value, err := normalizedSoulFromCommand(soulDefaultCreateCommand(raw))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("current builtin content: %s", soulmodel.StableSoulContentHash(value))
		for _, required := range []string{"sealed current failure", "currentValidationFailure", "failureDetailsIncomplete", "独立的正例与反例", "候选不能自行批准新的验收基线"} {
			if !strings.Contains(value.SoulMD, required) {
				t.Fatalf("execution guidance lost %s", required)
			}
		}
		return
	}
	t.Fatal("missing software-dev execution guidance")
}

type lostSoulUpgradeResponse struct {
	repository.SoulRepository
	updates int
}

func (r *lostSoulUpgradeResponse) Update(ctx context.Context, item *soulmodel.Soul) error {
	r.updates++
	if err := r.SoulRepository.Update(ctx, item); err != nil {
		return err
	}
	return errors.New("response lost after durable update")
}

func TestBuiltinSoulUpgradeIsExplicitGuardedAndReplayable(t *testing.T) {
	for _, backend := range []string{"memory", "file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var repo repository.SoulRepository = memoryrepo.NewSoulRepository()
			root := filepath.Join(t.TempDir(), "souls")
			if backend == "file" {
				repo = filerepo.NewSoulRepository(root)
			}
			s := newService(repo)
			old, err := s.createSoul(ctx, historicalExecutionSoul(t), soulmodel.SoulSourceBuiltinSeed)
			if err != nil {
				t.Fatal(err)
			}
			if err := ensureBuiltinSoulDefaults(ctx, s); err != nil {
				t.Fatal(err)
			}
			unchanged, err := s.getSoul(ctx, old.ID)
			if err != nil || !reflect.DeepEqual(unchanged, old) {
				t.Fatalf("startup upgraded existing content: %v", err)
			}
			preview, err := s.queryBuiltinDefaults(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var request soulBuiltinUpgradeEvent
			for _, item := range preview {
				if item.SoulID == old.ID {
					if !item.UpgradeAvailable || item.CurrentBuiltinRevision == "" {
						t.Fatalf("historical version not recognized: %+v", item)
					}
					request = soulBuiltinUpgradeEvent{SoulID: old.ID, ExpectedContentHash: item.CurrentContentHash, TargetContentHash: item.BuiltinContentHash}
				}
			}
			// The owner has committed, but the caller cannot know whether it did.
			lost := &lostSoulUpgradeResponse{SoulRepository: repo}
			if _, err := newService(lost).submitBuiltinDefaultUpgrade(ctx, request); err == nil {
				t.Fatal("expected lost response")
			}
			if lost.updates != 1 {
				t.Fatalf("writes = %d", lost.updates)
			}
			if backend == "file" {
				repo = filerepo.NewSoulRepository(root)
			}
			s = newService(repo)
			for attempt := 0; attempt < 2; attempt++ {
				result, err := s.submitBuiltinDefaultUpgrade(ctx, request)
				if err != nil || result.Upgraded || result.Default.UpgradeAvailable || result.Default.CurrentContentHash != request.TargetContentHash {
					t.Fatalf("replay after reload: %+v, %v", result, err)
				}
			}
			updated, err := s.getSoul(ctx, old.ID)
			if err != nil || updated.Revision != old.Revision+1 {
				t.Fatalf("upgrade revision: %+v, %v", updated, err)
			}
			// Ordinary editing retains builtin_seed provenance; it is NOT upgrade consent.
			custom := historicalExecutionSoul(t)
			custom.SoulMD += "\nUser-maintained instructions.\n"
			customized, err := s.updateSoul(ctx, custom)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.submitBuiltinDefaultUpgrade(ctx, request); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("stale upgrade after custom edit: %v", err)
			}
			got, err := s.getSoul(ctx, old.ID)
			if err != nil || !reflect.DeepEqual(got, customized) {
				t.Fatalf("custom edit overwritten: %v", err)
			}
		})
	}
}

func TestBuiltinSoulUpgradeRejectsUntrustedOrUnreviewedRequests(t *testing.T) {
	ctx := context.Background()
	repo := memoryrepo.NewSoulRepository()
	s := newService(repo)
	old, err := s.createSoul(ctx, historicalExecutionSoul(t), soulmodel.SoulSourceBuiltinSeed)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.queryBuiltinDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var target string
	for _, item := range items {
		if item.SoulID == old.ID {
			target = item.BuiltinContentHash
		}
	}
	valid := soulBuiltinUpgradeEvent{SoulID: old.ID, ExpectedContentHash: old.ContentHash, TargetContentHash: target}
	for _, change := range []func(*soulBuiltinUpgradeEvent){
		func(e *soulBuiltinUpgradeEvent) { e.ExpectedContentHash = "" },
		func(e *soulBuiltinUpgradeEvent) { e.ExpectedContentHash = "unknown" },
		func(e *soulBuiltinUpgradeEvent) { e.TargetContentHash = "different-release" },
		func(e *soulBuiltinUpgradeEvent) { e.SoulID = "missing" },
	} {
		request := valid
		change(&request)
		if _, err := s.submitBuiltinDefaultUpgrade(ctx, request); err == nil {
			t.Fatalf("invalid upgrade accepted: %+v", request)
		}
	}
	owner := &Biz{service: s}
	command := soulevents.UpgradeBuiltinSoulDefaultCommand{SoulID: valid.SoulID, ExpectedContentHash: valid.ExpectedContentHash, TargetContentHash: valid.TargetContentHash}
	for _, source := range []string{"untrusted/module", " " + applicationContextRegistrySource + " "} {
		event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandUpgradeBuiltinDefault, source, soulevents.CommandDestination, magicevent.NewHeader(), ctx, command)
		result := magicevent.NewResult(event.ID(), source, soulevents.CommandDestination)
		owner.handleBuiltinDefaultsCommand(event, result)
		if _, err := result.Get(); err == nil {
			t.Fatalf("unauthorized source accepted: %q", source)
		}
	}
	event := magicevent.NewEventWithContext(soulevents.TopicSoulCommandUpgradeBuiltinDefault, applicationContextRegistrySource, soulevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	owner.handleBuiltinDefaultsCommand(event, nil)
	got, err := s.getSoul(ctx, old.ID)
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatalf("rejected/Post requests wrote content: %v", err)
	}
	result, err := s.submitBuiltinDefaultUpgrade(ctx, valid)
	if err != nil || !result.Upgraded {
		t.Fatalf("valid request: %+v %v", result, err)
	}
	// A user-created resource with the same builtin ID/content is still user-owned.
	userRepo := memoryrepo.NewSoulRepository()
	userService := newService(userRepo)
	if _, err := userService.createSoulCommand(ctx, historicalExecutionSoul(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := userService.submitBuiltinDefaultUpgrade(ctx, valid); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("upgraded user-managed resource: %v", err)
	}
}

func TestBuiltinSoulUpgradeSerializesWithOrdinaryEdit(t *testing.T) {
	ctx := context.Background()
	s := newService(memoryrepo.NewSoulRepository())
	cmd := historicalExecutionSoul(t)
	old, err := s.createSoul(ctx, cmd, soulmodel.SoulSourceBuiltinSeed)
	if err != nil {
		t.Fatal(err)
	}
	items, err := s.queryBuiltinDefaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var request soulBuiltinUpgradeEvent
	for _, item := range items {
		if item.SoulID == old.ID {
			request = soulBuiltinUpgradeEvent{SoulID: old.ID, ExpectedContentHash: old.ContentHash, TargetContentHash: item.BuiltinContentHash}
		}
	}
	cmd.Description += " concurrent user edit"
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		if _, err := s.updateSoul(ctx, cmd); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wait.Done()
		<-start
		if _, err := s.submitBuiltinDefaultUpgrade(ctx, request); err != nil && !errors.Is(err, common.ErrConflict) {
			t.Error(err)
		}
	}()
	close(start)
	wait.Wait()
	got, err := s.getSoul(ctx, old.ID)
	if err != nil || got.Description != cmd.Description {
		t.Fatalf("upgrade overwrote concurrent edit: %+v %v", got, err)
	}
}
