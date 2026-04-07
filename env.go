package goenvars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/joho/godotenv"
)

var (
	loadEnvOnce      sync.Once
	mu               sync.RWMutex
	muLoad           sync.RWMutex
	awsSecrets       = make([]map[string]any, 0)
	awsSecretsLoaded = make(map[string]string)
	preloadedVars    = make(map[string]any)
	loaded           bool
)

type EnvDef struct {
	Key  string
	Type string
}

func loadVars() {
	loadEnvOnce.Do(func() {
		if err := godotenv.Load(); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Warning: archivo .env no encontrado (%v)", err)
			}

			log.Printf("Error cargando .env: %v", err)
		}
	})
}

func LoadAwsSecret(secretName, region string) error {
	mu.Lock()
	defer mu.Unlock()

	if secretName == "" || region == "" {
		fmt.Println("secret_name o aws_region no definidos para cargar secretos")
		return errors.New("secret_name o aws_region no definidos para cargar secretos")
	}

	_, ok := awsSecretsLoaded[secretName]
	if ok {
		fmt.Printf("secreto %s ya cargado, omitiendo\n", secretName)
		return nil
	}

	awsCfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		fmt.Printf("cargando configuración AWS: %v\n", err)
		return err
	}

	svc := secretsmanager.NewFromConfig(awsCfg)
	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := svc.GetSecretValue(context.TODO(), input)
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

	awsSecrets = append(awsSecrets, awsSecretTmp)
	awsSecretsLoaded[secretName] = secretName

	return nil
}

func LoadEnvVars() error {
	var retErr error
	loadVars()

	awsSecret := os.Getenv("AWS_SECRET_NAME")
	awsRegion := os.Getenv("AWS_REGION")
	if awsSecret != "" && awsRegion != "" {
		if err := LoadAwsSecret(awsSecret, awsRegion); err != nil {
			retErr = err
		}
	}

	return retErr
}

func PreloadEnvVars(vars []EnvDef) {
	muLoad.Lock()
	defer muLoad.Unlock()

	if loaded {
		return
	}

	for _, def := range vars {
		var rawvalue any
		value := os.Getenv(def.Key)

		if value == "" {
			awsValue, exito := getAwsValue(def.Key)
			if exito {
				rawvalue = awsValue
			} else {
				continue
			}
		} else {
			rawvalue = value
		}

		switch def.Type {
		case "bool":
			if vBool, ok := rawvalue.(bool); ok {
				preloadedVars[def.Key] = vBool
			} else {
				if b, err := strconv.ParseBool(fmt.Sprintf("%v", rawvalue)); err == nil {
					preloadedVars[def.Key] = b
				}
			}
		case "int":
			if vInt, ok := rawvalue.(int); ok {
				preloadedVars[def.Key] = vInt
			} else {
				if n, err := strconv.Atoi(fmt.Sprintf("%v", rawvalue)); err == nil {
					preloadedVars[def.Key] = n
				}
			}
		case "float64":
			if vFloat, ok := rawvalue.(float64); ok {
				preloadedVars[def.Key] = vFloat
			} else {
				if f, err := strconv.ParseFloat(fmt.Sprintf("%v", rawvalue), 64); err == nil {
					preloadedVars[def.Key] = f
				}
			}
		default:
			preloadedVars[def.Key] = rawvalue
		}
	}
	loaded = true
}

func Get[T any](key string, defaultValue T) T {
	muLoad.RLock()

	if val, exists := preloadedVars[key]; exists {
		if v, ok := val.(T); ok {
			muLoad.RUnlock()
			return v
		}
	}

	if loaded {
		muLoad.RUnlock()
		return defaultValue
	}
	muLoad.RUnlock()

	muLoad.Lock()
	defer muLoad.Unlock()

	if val, exists := preloadedVars[key]; exists {
		if v, ok := val.(T); ok {
			return v
		}
	}

	var rawvalue any
	val := os.Getenv(key)
	if val == "" {
		awsValue, exito := getAwsValue(key)
		if exito {
			rawvalue = awsValue
		} else {
			preloadedVars[key] = defaultValue
			return defaultValue
		}
	} else {
		rawvalue = val
	}

	var parsed any
	switch any(defaultValue).(type) {
	case bool:
		if vBool, ok := rawvalue.(bool); ok {
			parsed = vBool
		} else {
			if b, err := strconv.ParseBool(fmt.Sprintf("%v", rawvalue)); err == nil {
				parsed = b
			} else {
				parsed = defaultValue
			}
		}
	case int:
		if vInt, ok := rawvalue.(int); ok {
			parsed = vInt
		} else {
			if n, err := strconv.Atoi(fmt.Sprintf("%v", rawvalue)); err == nil {
				parsed = n
			} else {
				parsed = defaultValue
			}
		}
	case float64:
		if vFloat, ok := rawvalue.(float64); ok {
			parsed = vFloat
		} else {
			if f, err := strconv.ParseFloat(fmt.Sprintf("%v", rawvalue), 64); err == nil {
				parsed = f
			} else {
				parsed = defaultValue
			}
		}
	default:
		parsed = rawvalue
	}

	preloadedVars[key] = parsed

	return parsed.(T)
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

func getAwsValue(key string) (any, bool) {
	mu.RLock()
	defer mu.RUnlock()

	if awsSecrets == nil {
		return nil, false
	}

	for _, awsSecret := range awsSecrets {
		if awsValue, ok := awsSecret[key]; ok {
			return awsValue, true
		}
	}
	return nil, false
}
