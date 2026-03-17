# ibcq-source-k8s Constitution

## Project Overview

This is a CloudQuery source plugin for Kubernetes (K8s). The plugin fetches data from Kubernetes clusters and transforms it into Apache Arrow records for use with CloudQuery.

**Technology Stack**: Golang
**Architecture**: CloudQuery source plugin pattern
**Data Flow**: Kubernetes API → Plugin → Apache Arrow Records → CloudQuery

## Core Principles

### I. Test-Driven Development
All code must have comprehensive test coverage. Test classes are required for:
- Client implementations
- Resource fetchers
- Data transformers
- Integration tests with Kubernetes API

Tests must be written and validated before implementation.

### II. Kubernetes API Integration
- Use official Kubernetes Go client libraries
- Handle API versioning and deprecations
- Implement proper authentication mechanisms (kubeconfig, service accounts, etc.)
- Support multiple Kubernetes versions

### III. Apache Arrow Record Generation
- Transform Kubernetes resources into Apache Arrow format
- Maintain schema consistency and type safety
- Optimize for performance and memory efficiency
- Ensure proper serialization of complex Kubernetes objects

### IV. Resource Coverage
- Support core Kubernetes resources (Pods, Services, Deployments, etc.)
- Include Custom Resource Definitions (CRDs)
- Handle admission controllers and policies
- Maintain comprehensive table documentation

### V. Code Quality & Maintainability
- Follow Go best practices and idioms
- Use mockgen for test mocks
- Maintain clear separation between client, resources, and services
- Document all public APIs and exported functions

## Technical Constraints

### Dependencies
- Official Kubernetes client-go library
- Apache Arrow Go implementation
- CloudQuery SDK for plugin development
- Mockgen for generating test mocks

### Configuration
- Support YAML-based configuration
- Validate configuration schema
- Handle multiple cluster contexts
- Provide sensible defaults

### Performance
- Implement efficient pagination for large resource sets
- Use caching where appropriate
- Handle rate limiting from Kubernetes API
- Optimize memory usage for large clusters

## Development Workflow

### Testing Requirements
- Unit tests for all business logic
- Integration tests for Kubernetes API interactions
- Mock-based tests using mockgen
- Test coverage reports

### Code Organization
- `client/`: Kubernetes client and helper functions
- `resources/`: Resource definitions and fetchers
- `mocks/`: Generated mock implementations
- `policies/`: Query policies and compliance checks
- `docs/`: Table documentation and guides

## Governance

All changes must:
- Include appropriate tests
- Maintain backward compatibility where possible
- Update relevant documentation
- Follow existing code patterns and structure

**Version**: 1.0.0 | **Ratified**: 2026-02-27 | **Last Amended**: 2026-02-27
