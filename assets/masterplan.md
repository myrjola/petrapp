# Petrapp: Personal Trainer App Masterplan

## Overview

Petrapp is a minimalist, privacy-focused mobile web application that automatically generates and tracks gym workouts. The app emphasizes simplicity in both user experience and technical implementation, using standard libraries and minimal dependencies.

## Core Principles

- Always prefer simple, elegant solutions (KISS principle).
- Avoid duplication of code (DRY principle); check existing codebase first.
- Only add functionality when explicitly needed (YAGNI principle).
- Adhere to SOLID principles where applicable (e.g., single responsibility, dependency inversion).
- Keep code clean, organized, and under 200-300 lines per file; refactor proactively.

## Target Audience

- People wanting to get into better shape
- Users who prefer guided workouts over creating their own
- Privacy-conscious individuals
- Mobile device users

## Core Features

### User Management

- Anonymous authentication using WebAuthn/Passkeys
- Account deletion capability for privacy
- No username/password management required

### Workout Planning

- Weekly schedule selection
- Automatic workout generation based on:
    - Selected workout days
    - Exercise split logic (upper/lower body for consecutive days)
    - Full body workouts for non-consecutive days
- Exercise progression based on user performance

### Workout Execution

- Set-by-set tracking of:
    - Weight used
    - Reps completed
    - Target rep ranges
- Post-workout difficulty feedback
- Real-time progress saving
- Ability to complete workout even with incomplete sets

## Technical Architecture

### Backend

- Go with standard library focus and minimal dependencies
- SQLite database for simple, self-contained data storage
    - Since the data is local, n+1 query problem is not a concern. Keep the SQL queries simple.

### Frontend

- No React or other frontend frameworks
- Go templates for HTML generation (template/html standard library packge)
- Vanilla JavaScript without build system
- Minimalist styling using minimal CSS while maintaining accessibility and usability
- Scoped CSS for style isolation
- Mobile-only interface

## Task Execution

- Focus only on code relevant to the task; you never touch unrelated code.
- Break complex tasks into logical stages; pause and ask for confirmation before next step.
- For simple, low-risk tasks, implement fully; for complex tasks, use review checkpoints.
- Before major features, generate plan.md with steps and wait for my approval.
