#!/bin/bash

# Git Hooks Setup Script for Petrapp

chmod +x .githooks/*
git config core.hooksPath .githooks

echo "Git hooks for Petrapp have been set up successfully!"
