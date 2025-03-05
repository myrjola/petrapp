# Petrapp: Personal Trainer App Masterplan

## Overview
Petrapp is a minimalist, privacy-focused mobile web application that automatically generates and tracks gym workouts. The app emphasizes simplicity in both user experience and technical implementation, using standard libraries and minimal dependencies.

## Core Principles
- Minimalism in design and implementation
- Strong privacy focus with anonymous users
- Mobile-first interface
- Standard library usage over external dependencies
- Simple deployment and maintenance

## Target Audience
- People wanting to get into better shape
- Users who prefer guided workouts over creating their own
- Privacy-conscious individuals
- Mobile device users

## Core Features

### User Management
- Anonymous authentication using WebAuthn/Passkeys
- Cross-device access support
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

### Future Features
- Exercise descriptions and form tips
- Alternative exercise suggestions based on equipment availability
- Enhanced exercise database with more detailed information

## Technical Architecture

### Backend
- Go with standard library focus
- SQLite database for simple, self-contained data storage
- Minimal external dependencies
- Container-based deployment to fly.io

### Frontend
- Go templates for HTML generation
- Vanilla JavaScript without build system
- No React or other frontend frameworks
- Minimalist styling using minimal CSS while maintaining accessibility and usability
- Scoped CSS for style isolation
- Mobile-only interface
- PWA support for home screen installation

### Data Model

#### Core Entities:
1. Users
    - Anonymous identifier
    - WebAuthn credentials
    - Weekly preferences

2. Exercises
    - Name
    - Category (upper body, lower body, full body)
    - Target muscle groups

3. Workouts
    - Date
    - Exercise sets
    - Completion status
    - Difficulty rating

4. Sets
    - Weight
    - Target rep range
    - Completed reps
    - Status

### Security & Privacy Features
- Passwordless authentication
- Minimal data collection
- No user identifiable information
- Data deletion capability
- No third-party dependencies that could compromise privacy

## Development Phases

### Phase 1: Core Functionality (Current)
- [x] User authentication
- [X] Weekly schedule management
- [X] Basic workout generation
- [X] Set tracking
- [ ] Progress tracking

### Phase 2: Enhancement
- Exercise database expansion
- Form tips and descriptions
- Alternative exercise suggestions
- Progress visualization

### Phase 3: User Experience
- Enhanced mobile experience
- Progress milestones
- Performance optimizations

## Technical Considerations

### Data Storage
- SQLite provides a simple, self-contained database solution
- Minimal data model focused on essential workout information
- No need for complex queries or relationships

### Scalability
- Current architecture suitable for initial user base
- Container-based deployment provides flexibility
- SQLite limitations understood and accepted for MVP

### Challenges and Solutions

1. Workout Generation Algorithm
    - Challenge: Creating appropriate workout splits
    - Solution: Simple rules based on consecutive days
    - Future: Enhanced logic based on user feedback

2. Progress Tracking
    - Challenge: Determining appropriate weight/rep progressions
    - Solution: Start with basic progression model
    - Future: More sophisticated progression based on user data

3. Mobile Experience
    - Challenge: Ensuring smooth mobile-only experience
    - Solution: Focus on mobile-first design
    - PWA implementation for home screen access

## Future Considerations
- Enhanced exercise database
- More sophisticated workout generation
- Advanced progress tracking
- Performance optimization if needed
- Potential backup/export features

This masterplan will be revised as the project evolves and new requirements emerge.
