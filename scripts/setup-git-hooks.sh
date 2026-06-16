#!/bin/bash

# Git Hooks Setup Script for Petra

chmod +x .githooks/*
git config core.hooksPath .githooks

echo "Git hooks for Petra have been set up successfully!"
