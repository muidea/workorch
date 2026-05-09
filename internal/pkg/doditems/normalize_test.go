package doditems

import (
	"reflect"
	"testing"
)

func TestNormalizeKeepsSimpleItems(t *testing.T) {
	got := Normalize([]string{"first item", " second item ", "", "first item"})
	want := []string{"first item", "second item"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalizeSplitsInlineSimpleItems(t *testing.T) {
	got := Normalize([]string{"first item; second item；third item"})
	want := []string{"first item", "second item", "third item"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalizeMarkdownDoDKeepsLogicalLeafItems(t *testing.T) {
	got := Normalize([]string{`
### 3.1 Functional
- [ ] **Forwarding**: the request reaches the configured upstream
- [ ] **Capture**:
  - response metadata is persisted with the run
  - related evidence is linked to the acceptance item

### 3.2 Validation
- [ ] final result references the produced artifact
`})
	want := []string{
		"Functional： Forwarding: the request reaches the configured upstream",
		"Functional / Capture： response metadata is persisted with the run",
		"Functional / Capture： related evidence is linked to the acceptance item",
		"Validation： final result references the produced artifact",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalizeDoesNotSplitStructuredLineWithSemicolon(t *testing.T) {
	got := Normalize([]string{"- [ ] item keeps semicolon; inside the same acceptance text"})
	want := []string{"item keeps semicolon; inside the same acceptance text"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}

func TestNormalizeSameLevelParentAppliesUntilNextLabeledItem(t *testing.T) {
	got := Normalize([]string{`
### 3.1 Functional
- [ ] **Forwarding**: request reaches the configured target
- [ ] **Capture**:
- [ ] first captured value is stored
- [ ] second captured value is linked to evidence
- [ ] **Error handling**: upstream errors are visible to the caller
`})
	want := []string{
		"Functional： Forwarding: request reaches the configured target",
		"Functional / Capture： first captured value is stored",
		"Functional / Capture： second captured value is linked to evidence",
		"Functional： Error handling: upstream errors are visible to the caller",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Normalize() = %#v, want %#v", got, want)
	}
}
