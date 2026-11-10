package goenvars

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reset(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_SECRET_NAME", "") // evita llamadas reales a AWS si el entorno las define
	envLoaded.Store(false)
	muLoad.Lock()
	preloadedVars = map[string]any{}
	muLoad.Unlock()
}

// unset deja la variable sin definir y la restaura al terminar el test.
func unset(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	os.Unsetenv(key)
}

func dirConEnv(t *testing.T, nombre, contenido string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, nombre), []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

func TestCargaAutomaticaDeEnv(t *testing.T) {
	reset(t)
	unset(t, "MI_VAR")
	dirConEnv(t, ".env", "MI_VAR=desde_env\n")
	if got := GetEnv("MI_VAR", "def"); got != "desde_env" {
		t.Fatalf("got %q", got)
	}
}

func TestDefaultNoSeMemoriza(t *testing.T) {
	reset(t)
	unset(t, "X_NUEVA")
	dirConEnv(t, ".env", "")
	if a, b := GetEnv("X_NUEVA", "A"), GetEnv("X_NUEVA", "B"); a != "A" || b != "B" {
		t.Fatalf("got %q %q", a, b)
	}
	t.Setenv("X_NUEVA", "v")
	if got := GetEnv("X_NUEVA", "A"); got != "v" {
		t.Fatalf("got %q", got)
	}
}

func TestCambioEnCaliente(t *testing.T) {
	reset(t)
	dirConEnv(t, ".env", "")
	t.Setenv("K", "uno")
	a := GetEnv("K", "")
	t.Setenv("K", "dos")
	if b := GetEnv("K", ""); a != "uno" || b != "dos" {
		t.Fatalf("got %q %q", a, b)
	}
}

func TestArchivoExplicito(t *testing.T) {
	reset(t)
	unset(t, "DE_PROD")
	dirConEnv(t, ".env.prod", "DE_PROD=si\n")
	if err := LoadEnvVars(".env.prod"); err != nil { // antes de cualquier lectura: funciona
		t.Fatal(err)
	}
	if got := GetEnv("DE_PROD", ""); got != "si" {
		t.Fatalf("got %q", got)
	}
}

func TestArchivoExplicitoTrasLecturaTemprana(t *testing.T) {
	reset(t)
	dirConEnv(t, ".env", "")
	GetEnv("CUALQUIERA", "") // lectura temprana: dispara la carga automática
	if err := LoadEnvVars(".env.prod"); err == nil {
		t.Fatal("se esperaba error y no hubo")
	}
}

func TestArchivoExplicitoInexistente(t *testing.T) {
	reset(t)
	dirConEnv(t, ".env", "")
	if err := LoadEnvVars("no_existe.env"); err == nil {
		t.Fatal("se esperaba error")
	}
}

func TestSecretoNumericoComoString(t *testing.T) { // antes: panic
	reset(t)
	dirConEnv(t, ".env", "")
	unset(t, "N_AWS")
	mu.Lock()
	awsSecrets = append(awsSecrets, map[string]any{"N_AWS": float64(1000000)})
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); awsSecrets = nil; mu.Unlock() })
	if got := GetEnv("N_AWS", ""); got != "1000000" {
		t.Fatalf("got %q", got)
	}
}

type testSource map[string]string

func (s testSource) Lookup(key string) (string, bool) { v, ok := s[key]; return v, ok }

func TestAddSource(t *testing.T) {
	reset(t)
	unset(t, "DE_SOURCE")
	AddSource(testSource{"DE_SOURCE": "desde_source"})
	t.Cleanup(func() {
		mu.Lock()
		sources = nil
		mu.Unlock()
	})
	if got := GetEnv("DE_SOURCE", "def"); got != "desde_source" {
		t.Fatalf("got %q", got)
	}
}

type reentrante struct{}

func (reentrante) Lookup(k string) (string, bool) {
	if k == "OTRA" { // evita recursión infinita: solo la clave original dispara la relectura
		return "", false
	}
	GetEnv("OTRA", "")
	return "x", true
}

func TestPreloadConSourceReentrante(t *testing.T) { // antes: bloqueo mutuo
	reset(t)
	dirConEnv(t, ".env", "")
	unset(t, "R1")
	AddSource(reentrante{})
	t.Cleanup(func() { mu.Lock(); sources = nil; mu.Unlock() })

	done := make(chan struct{})
	go func() { PreloadEnvVars([]EnvDef{{Key: "R1"}}); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("PreloadEnvVars se bloqueó")
	}
}

func TestReintentoTrasErrorExplicito(t *testing.T) {
	reset(t)
	unset(t, "ZZ_PROD")
	dirConEnv(t, ".env.prod", "ZZ_PROD=1\n")
	if err := LoadEnvVars("typo.env"); err == nil {
		t.Fatal("se esperaba error")
	}
	if err := LoadEnvVars(".env.prod"); err != nil {
		t.Fatalf("el reintento no debería bloquearse: %v", err)
	}
}

func TestSecretoLegacyNullYAnidado(t *testing.T) {
	reset(t)
	dirConEnv(t, ".env", "")
	unset(t, "L_NULL")
	unset(t, "L_OBJ")
	mu.Lock()
	awsSecrets = append(awsSecrets, map[string]any{"L_NULL": nil, "L_OBJ": map[string]any{"a": float64(1)}})
	mu.Unlock()
	t.Cleanup(func() { mu.Lock(); awsSecrets = nil; mu.Unlock() })

	if got := GetEnv("L_NULL", "def"); got != "def" {
		t.Fatalf("null debería ser ausente, got %q", got)
	}
	if got := GetEnv("L_OBJ", ""); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}
