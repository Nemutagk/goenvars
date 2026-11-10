package goenvars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

var (
	envMu            sync.Mutex
	envLoaded        atomic.Bool // true tras el primer intento de cargar .env
	mu               sync.RWMutex
	muLoad           sync.RWMutex
	awsSecrets       = make([]map[string]any, 0)
	awsSecretsLoaded = make(map[string]string)
	preloadedVars    = make(map[string]any)
)

type EnvDef struct {
	Key  string
	Type string
}

// loadVars carga .env una sola vez. Sin archivos: .env por defecto, y si falta solo avisa.
// Con archivos explícitos: si ya hubo una carga previa devuelve error en vez de ignorarlos.
func loadVars(files ...string) error {
	if len(files) == 0 && envLoaded.Load() {
		return nil // camino rápido de Get
	}

	envMu.Lock()
	defer envMu.Unlock()

	if envLoaded.Load() {
		if len(files) > 0 {
			return errors.New("goenvars: las variables ya se cargaron; LoadEnvVars con archivos debe llamarse antes de cualquier lectura")
		}
		return nil
	}

	err := godotenv.Load(files...)
	if err != nil && len(files) > 0 {
		return err // sin marcar como cargado: permite reintentar con otro archivo
	}
	envLoaded.Store(true)

	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("Warning: archivo .env no encontrado (%v)", err)
			return nil
		}
		return err
	}
	return nil
}

// Deprecated: usar el módulo goenvars/aws con AddSource.
func LoadAwsSecret(secretName, region string) error {
	if secretName == "" || region == "" {
		fmt.Println("secret_name o aws_region no definidos para cargar secretos")
		return errors.New("secret_name o aws_region no definidos para cargar secretos")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mu.RLock()
	_, ok := awsSecretsLoaded[secretName]
	mu.RUnlock()
	if ok {
		fmt.Printf("secreto %s ya cargado, omitiendo\n", secretName)
		return nil
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		fmt.Printf("cargando configuración AWS: %v\n", err)
		return err
	}

	svc := secretsmanager.NewFromConfig(awsCfg)
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := svc.GetSecretValue(ctx, input)
	if err != nil {
		fmt.Printf("obteniendo secreto: %v\n", err)
		return err
	}

	if result.SecretString == nil {
		fmt.Printf("SecretString vacío para %s\n", secretName)
		return fmt.Errorf("SecretString vacío para %s", secretName)
	}

	var awsSecretTmp map[string]any
	if err := json.Unmarshal([]byte(*result.SecretString), &awsSecretTmp); err != nil {
		fmt.Printf("unmarshal secreto: %v\n", err)
		return fmt.Errorf("unmarshal secreto: %w", err)
	}

	mu.Lock()
	awsSecrets = append(awsSecrets, awsSecretTmp)
	awsSecretsLoaded[secretName] = secretName
	mu.Unlock()

	return nil
}

func LoadEnvVars(files ...string) error {
	if err := loadVars(files...); err != nil {
		return err
	}

	awsSecret := os.Getenv("AWS_SECRET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	if awsSecret != "" && awsRegion != "" {
		return LoadAwsSecret(awsSecret, awsRegion)
	}
	return nil
}

// Source es una fuente externa de variables (AWS, Vault, SSM...). Devuelve texto;
// goenvars hace la conversión de tipo.
//
// Lookup debe ser rápido y no llamar a goenvars (Get/GetEnv...): carga los datos al
// construir la fuente y aquí solo consulta. Si no, puede haber recursión infinita.
type Source interface {
	Lookup(key string) (string, bool)
}

var sources []Source // protegido por mu

// AddSource registra una fuente; se consulta después del entorno, en orden de registro.
// Llamarla antes de PreloadEnvVars.
func AddSource(s Source) {
	mu.Lock()
	defer mu.Unlock()
	sources = append(sources, s)
}

func PreloadEnvVars(vars []EnvDef) {
	if err := loadVars(); err != nil {
		log.Printf("Error cargando .env: %v", err)
	}

	// Se resuelve fuera del candado: lookup ejecuta código de las fuentes (puede llamar a Get o tardar).
	resultado := make(map[string]any, len(vars))
	for _, def := range vars {
		raw, found := lookup(def.Key)
		if !found {
			continue
		}

		var v any
		var ok bool
		switch def.Type {
		case "bool":
			v, ok = parse(raw, false)
		case "int":
			v, ok = parse(raw, 0)
		case "float64":
			v, ok = parse(raw, 0.0)
		default:
			v, ok = parse(raw, "")
		}
		if !ok {
			log.Printf("goenvars: %s no se pudo convertir a %q", def.Key, def.Type)
			continue
		}
		resultado[def.Key] = v
	}

	muLoad.Lock()
	for k, v := range resultado {
		preloadedVars[k] = v
	}
	muLoad.Unlock()
}

func Get[T any](key string, defaultValue T) T {
	if err := loadVars(); err != nil {
		log.Printf("Error cargando .env: %v", err)
	}

	// Solo PreloadEnvVars escribe en este mapa, y es explícito.
	muLoad.RLock()
	val, exists := preloadedVars[key]
	muLoad.RUnlock()
	if exists {
		if v, ok := val.(T); ok {
			return v
		}
	}

	raw, found := lookup(key)
	if !found {
		return defaultValue
	}
	if v, ok := parse(raw, defaultValue); ok {
		return v
	}
	return defaultValue
}

func GetEnv(key string, defaultValue string) string {
	return Get(key, defaultValue)
}

func GetEnvBool(key string, defaultValue bool) bool {
	return Get(key, defaultValue)
}

func GetEnvInt(key string, defaultValue int) int {
	return Get(key, defaultValue)
}

func GetEnvFloat(key string, defaultValue float64) float64 {
	return Get(key, defaultValue)
}

// lookup: primero el entorno (aunque esté vacío), luego los sources registrados,
// luego los secretos de AWS.
func lookup(key string) (any, bool) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}

	// Copia: no se ejecuta código de las fuentes con mu tomado.
	mu.RLock()
	ss := slices.Clone(sources)
	mu.RUnlock()

	for _, s := range ss {
		if v, ok := s.Lookup(key); ok {
			return v, true
		}
	}

	return getAwsValue(key)
}

// parse convierte raw al tipo de def. Si no se puede, devuelve ok=false (nunca hace panic).
func parse[T any](raw any, def T) (T, bool) {
	s := fmt.Sprintf("%v", raw)
	switch x := raw.(type) {
	case float64: // JSON de AWS: evita "1e+06"
		s = strconv.FormatFloat(x, 'f', -1, 64)
	case map[string]any, []any: // JSON anidado: texto JSON, no "map[a:1]"
		b, _ := json.Marshal(x)
		s = string(b)
	}

	var out any
	var err error
	switch any(def).(type) {
	case string:
		out = s
	case bool:
		out, err = strconv.ParseBool(s)
	case int:
		out, err = strconv.Atoi(s)
	case float64:
		out, err = strconv.ParseFloat(s, 64)
	default:
		return def, false // shortcut: solo string/bool/int/float64, ampliar si hace falta
	}

	v, ok := out.(T)
	return v, err == nil && ok
}

func getAwsValue(key string) (any, bool) {
	mu.RLock()
	defer mu.RUnlock()

	if awsSecrets == nil {
		return nil, false
	}

	for _, awsSecret := range awsSecrets {
		if awsValue, ok := awsSecret[key]; ok && awsValue != nil { // null = ausente
			return awsValue, true
		}
	}
	return nil, false
}
