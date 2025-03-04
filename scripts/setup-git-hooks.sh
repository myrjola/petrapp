#!/bin/bash

# Git Hooks Setup Script for Petrapp

# Create .githooks directory if it doesn't exist
mkdir -p .githooks

# Pre-push hook
cat > .githooks/pre-push << 'EOL'
#!/bin/bash

if ! git diff --quiet; then
    echo "Error: You have unstaged changes."
    echo "Please stage or stash your changes before pushing."
    exit 1
fi

# Check for staged but not committed changes
if ! git diff --cached --quiet; then
    echo "Error: You have staged changes that are not committed."
    echo "Please commit your changes before pushing."
    exit 1
fi

# Check for untracked files
if [ -n "$(git ls-files --others --exclude-standard)" ]; then
    echo "Error: You have untracked files."
    echo "Please add, commit, or ignore these files."
    echo "Untracked files:"
    git ls-files --others --exclude-standard
    exit 1
fi

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
