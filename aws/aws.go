package goenvarsaws

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

type source map[string]string

func (s source) Lookup(key string) (string, bool) { v, ok := s[key]; return v, ok }

// New descarga el secreto una vez y lo expone como goenvars.Source.
func New(ctx context.Context, secretName, region string) (source, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("cargando configuración AWS: %w", err)
	}

	out, err := secretsmanager.NewFromConfig(cfg).GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(secretName),
		VersionStage: aws.String("AWSCURRENT"),
	})
	if err != nil {
		return nil, fmt.Errorf("obteniendo secreto: %w", err)
	}
	if out.SecretString == nil {
		return nil, fmt.Errorf("SecretString vacío para %s", secretName)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(*out.SecretString), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal secreto: %w", err)
	}

	return flatten(raw), nil
}

// flatten convierte el JSON del secreto a texto. null = ausente; objetos y arreglos se dejan como JSON.
func flatten(raw map[string]any) source {
	s := make(source, len(raw))
	for k, v := range raw {
		switch x := v.(type) {
		case nil:
			continue
		case string:
			s[k] = x
		case float64:
			s[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			s[k] = strconv.FormatBool(x)
		default:
			b, _ := json.Marshal(x)
			s[k] = string(b)
		}
	}
	return s
}
