# Example Functions

This directory contains example functions to test the Docker FaaS platform.

## Available Examples

### 1. hello-world
A simple Python function that echoes input with a greeting.

**Build:**
```bash
cd hello-world
docker build -t docker-faas/hello-world:latest .
```

**Deploy:**
```bash
faas-cli deploy -f stack.yml --filter hello-world --gateway http://localhost:8080
```

**Test:**
```bash
echo "test" | faas-cli invoke hello-world --gateway http://localhost:8080
```

### 2. echo
A simple function using Alpine that echoes input using `cat`.

**Deploy:**
```bash
faas-cli deploy -f stack.yml --filter echo --gateway http://localhost:8080
```

**Test:**
```bash
echo "Hello World" | faas-cli invoke echo --gateway http://localhost:8080
```

### 3. env-test
Shows environment variables available to functions.

**Deploy:**
```bash
faas-cli deploy -f stack.yml --filter env-test --gateway http://localhost:8080
```

**Test:**
```bash
faas-cli invoke env-test --gateway http://localhost:8080
```

## Using Stack File

Deploy all functions at once:

```bash
faas-cli deploy -f stack.yml --gateway http://localhost:8080
```

List functions:

```bash
faas-cli list --gateway http://localhost:8080
```

Remove all functions:

```bash
faas-cli remove -f stack.yml --gateway http://localhost:8080
```

## Authentication

If authentication is enabled (default), login first:

```bash
faas-cli login --gateway http://localhost:8080 --username admin --password admin
```
