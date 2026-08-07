package credentials

import "sort"

// Fake is an in-memory Keyring for tests and for dry runs. It is deliberately
// exported so other packages can exercise credential handling without
// touching the developer's real keychain.
type Fake struct {
	Values map[string]string
	// Fail, when set, is returned by every operation, for testing error paths.
	Fail error
}

// NewFake builds an empty in-memory keyring.
func NewFake() *Fake { return &Fake{Values: map[string]string{}} }

func (f *Fake) Name() string    { return "in-memory (test)" }
func (f *Fake) Available() bool { return true }

func (f *Fake) Get(key string) (string, error) {
	if f.Fail != nil {
		return "", f.Fail
	}
	v, ok := f.Values[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *Fake) Set(key, value string) error {
	if f.Fail != nil {
		return f.Fail
	}
	f.Values[key] = value
	return nil
}

func (f *Fake) Delete(key string) error {
	if f.Fail != nil {
		return f.Fail
	}
	delete(f.Values, key)
	return nil
}

// Keys lists what is stored, sorted, for assertions.
func (f *Fake) Keys() []string {
	out := make([]string, 0, len(f.Values))
	for k := range f.Values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
