package events

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestConcreteFileInspectionOperationData(t *testing.T) {
	path := "internal/main.go"
	recursive := true
	maxEntries := int64(25)
	list, err := FileListOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "list-1", Kind: runtimecontract.StepKindFileList,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &path}, {Name: "recursive", Boolean: &recursive}, {Name: "maxEntries", Integer: &maxEntries},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if list.Path != path || !list.Recursive || list.MaxEntries != maxEntries {
		t.Fatalf("file/list data = %+v", list)
	}

	optional := true
	maxBytes := int64(4096)
	startLine := int64(10)
	endLine := int64(20)
	read, err := FileReadOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "read-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "path", String: &path}, {Name: "optional", Boolean: &optional}, {Name: "maxBytes", Integer: &maxBytes},
			{Name: "startLine", Integer: &startLine}, {Name: "endLine", Integer: &endLine},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if read.Path != path || !read.Optional || read.MaxBytes != maxBytes || read.StartLine != startLine || read.EndLine != endLine {
		t.Fatalf("file/read data = %+v", read)
	}
}

func TestConcretePatchPreviewOperationData(t *testing.T) {
	target := "README.md"
	patchType := "file_write"
	content := "typed preview\n"
	data, err := PatchPreviewOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "preview-1", Kind: runtimecontract.RuntimeBlockKindPatchPreview, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "targetRef", String: &target}, {Name: "patchType", String: &patchType}, {Name: "content", String: &content},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.TargetRef != target || data.PatchType != patchType || data.Content != content || !data.ContentPresent {
		t.Fatalf("patch/preview data = %+v", data)
	}
}

func TestConcreteSSHandWebOperationData(t *testing.T) {
	host := "example.internal"
	username := "operator"
	password := "secret"
	command := "uname -a"
	sshData, err := SSHOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "ssh-1", Kind: runtimecontract.StepKindSSHCommand,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "host", String: &host}, {Name: "username", String: &username}, {Name: "password", String: &password}, {Name: "command", String: &command},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sshData.Host != host || sshData.Username != username || sshData.Password != password || sshData.Command != command {
		t.Fatalf("ssh data = %+v", sshData)
	}

	query := "typed event contracts"
	maxResults := int64(5)
	webData, err := WebOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "web-1", Kind: runtimecontract.StepKindWebSearch,
		Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}, {Name: "maxResults", Integer: &maxResults}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if webData.Query != query || webData.MaxResults != maxResults || !webData.SafeSearch {
		t.Fatalf("web data = %+v", webData)
	}
}

func TestShellManagedValidationSessionOperationData(t *testing.T) {
	command := "go"
	arg := "test"
	timeout := int64(5000)
	scratch := "/tmp/validation-scratch"
	autoCleanup := true
	data, err := ShellOperationDataFromRuntimeOperation(runtimecontract.RuntimeOperation{
		ID: "validate-1", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		Parameters: []runtimecontract.OperationParameter{
			{Name: "command", String: &command},
			{Name: "args", StringList: []string{arg, "./..."}},
			{Name: "timeoutMillis", Integer: &timeout},
			{Name: "validationScratchDir", String: &scratch},
			{Name: "validationScratchAutoCleanup", Boolean: &autoCleanup},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Command != command || len(data.Args) != 2 || data.Args[0] != arg || data.TimeoutMillis != timeout {
		t.Fatalf("managed session shell data = %+v", data)
	}
	if data.ValidationScratchDirectory != scratch || !data.ValidationScratchAutoCleanup || !data.ValidationScratchAutoCleanupSet {
		t.Fatalf("managed session scratch = %+v", data)
	}
	if !data.ManagedValidation.RuntimeManaged || data.ManagedValidation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession || !data.ManagedValidation.AutoCleanup {
		t.Fatalf("managed validation metadata = %+v", data.ManagedValidation)
	}
}
