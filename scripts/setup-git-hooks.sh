#!/bin/bash

# Git Hooks Setup Script for Petrapp

# Create .githooks directory if it doesn't exist
mkdir -p .githooks

# Pre-push hook
cat > .githooks/pre-push << 'EOL'
#!/bin/bash

echo "Running linting and testing..."
make -j2 lint test

# Check the exit status of the tests
if [ $? -ne 0 ]; then
    echo "Verification failed. Push aborted."
    exit 1
fi
EOL

# Make hooks executable
chmod +x .githooks/pre-push

# Configure git to use these hooks
git config core.hooksPath .githooks

echo "Git hooks for Petrapp have been set up successfully!"
