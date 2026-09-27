package llm_test

import (
	"testing"

	"github.com/vanpiyp/awp/internal/llm"
)

func TestMustRegisterPanicsOnError(t *testing.T) {
	r := llm.NewRegistry()
	defer func() {
		if recover() == nil {
			t.Error("expected panic from MustRegister with empty id")
		}
	}()
	r.MustRegister("", &stubProvider{})
}

func TestMustRegisterDefaultPanicsOnError(t *testing.T) {
	r := llm.NewRegistry()
	defer func() {
		if recover() == nil {
			t.Error("expected panic from MustRegisterDefault when default already exists")
		}
	}()
	r.MustRegisterDefault(&stubProvider{})
	r.MustRegisterDefault(&stubProvider{})
}

func TestRegistryFindModelSkipsUnregistered(t *testing.T) {
	r := llm.NewRegistry()
	_ = r.Register("a", &stubProvider{models: []llm.Model{{ID: "a-m"}}})

	m, _, ok := r.FindModel("a-m")
	if !ok || m.ID != "a-m" {
		t.Errorf("FindModel(a-m) = (%v, _, %v), want (a-m, _, true)", m, ok)
	}
}

func TestRegistryAllModelsEmpty(t *testing.T) {
	r := llm.NewRegistry()
	if got := r.AllModels(); len(got) != 0 {
		t.Errorf("AllModels() = %v, want empty", got)
	}
}

func TestRegistryNamesEmpty(t *testing.T) {
	r := llm.NewRegistry()
	names := r.Names()
	if len(names) != 0 {
		t.Errorf("Names() = %v, want empty", names)
	}
}
