#!/bin/bash
set -e

echo "🔧 Initializing Git Repository for Docker FaaS"
echo "=============================================="
echo ""

# Check if git is installed
if ! command -v git &> /dev/null; then
    echo "❌ Git not found. Please install Git first."
    exit 1
fi

# Initialize git if not already initialized
if [ ! -d .git ]; then
    echo "📦 Initializing git repository..."
    git init
    echo "✅ Git repository initialized"
else
    echo "ℹ️  Git repository already exists"
fi

echo ""
echo "📝 Setting up git configuration..."

# Create .gitattributes for line endings
cat > .gitattributes << 'EOF'
# Auto detect text files and perform LF normalization
* text=auto

# Shell scripts
*.sh text eol=lf

# Go source files
*.go text eol=lf

# Documentation
*.md text eol=lf

# YAML files
*.yml text eol=lf
*.yaml text eol=lf

# Makefiles
Makefile text eol=lf

# Docker files
Dockerfile text eol=lf
.dockerignore text eol=lf
EOF

echo "✅ Created .gitattributes"

echo ""
echo "📋 Staging files..."
git add .

echo ""
echo "💬 Creating initial commit..."
git commit -m "Initial commit: Docker FaaS v1.0.0

- Implement OpenFaaS-compatible gateway API
- Add Docker provider for container management
- Add function router with load balancing
- Add SQLite state store
- Implement authentication and security
- Add Prometheus metrics
- Include comprehensive tests
- Add complete documentation
- Configure CI/CD pipelines
- Add example functions and deployment configs

🎉 Ready for production deployment!"

echo ""
echo "✅ Initial commit created!"
echo ""

# Optionally add remote
read -p "Would you like to add a remote repository? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    read -p "Enter remote repository URL: " REMOTE_URL
    if [ ! -z "$REMOTE_URL" ]; then
        git remote add origin "$REMOTE_URL"
        echo "✅ Remote added: $REMOTE_URL"
        echo ""
        echo "To push to remote, run:"
        echo "  git push -u origin main"
    fi
fi

echo ""
echo "🎯 Next steps:"
echo "  1. Review the commit: git log"
echo "  2. Create a GitHub repository (if not done)"
echo "  3. Push to remote: git push -u origin main"
echo "  4. Set up branch protection rules"
echo "  5. Enable GitHub Actions"
echo "  6. Add repository secrets for CI/CD"
echo ""
echo "✅ Git repository ready!"
echo ""
