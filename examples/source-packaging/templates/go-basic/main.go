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
        fmt.Println("go-basic: hello")
        return
    }
    fmt.Printf("go-basic: %s
", payload)
}
