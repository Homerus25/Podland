---
title: Create DB Initialization Command Program
status: completed
tags: [completed]
---

## What to build

Create a standalone Go command-line program that can initialize the database with a default admin user. The program should:

- Accept username and password as command-line arguments
- Connect to the SQLite database
- Create the default admin user with secure password hashing
- Be invokable via a Makefile target

## Acceptance criteria

- [ ] New Go program created at `backend/cmd/initdb/main.go`
- [ ] Command-line argument parsing for username and password
- [ ] Database connection logic
- [ ] Makefile target `make init-db` that runs the program
- [ ] Basic error handling for database operations

## Blocked by

None - can start immediately