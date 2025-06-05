package helper

import (
	"encoding/json"
	"fmt"
)

func PrettyPrint(data any) {
	// Convertir el mapa a JSON con sangría
	prettyJSON, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Println("Error formatting JSON:", err)
		return
	}

	// Imprimir el JSON formateado
	fmt.Println(string(prettyJSON))
}
