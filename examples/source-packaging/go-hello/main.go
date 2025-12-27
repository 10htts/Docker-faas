package main

import (
    "fmt"
    "io"
    "os"
    "strings"
)

func main() {
    data, _ := io.ReadAll(os.Stdin)
    payload := strings.TrimSpace(string(data))
    if payload == "" {
        fmt.Println("Hello from docker-faas (go). No input provided.")
        return
    }
    fmt.Printf("Hello from docker-faas (go). Input: %s
", payload)
}
