# Contributing to ondgo-products Workflows

Thank you for considering contributing to the ondgo-products GitHub Actions workflows! Your contributions help keep our CI/CD pipelines secure, maintainable, and extensible across all projects.

---

## How This Repository Works

This repository hosts reusable **GitHub composite actions** that provide:

* Secure authentication via Vault and Google Cloud
* Language-specific build templates (e.g., .NET, Nx)
* Docker image build and publish workflows
* GitOps repo updates for ArgoCD deployments

Each workflow is designed to be modular and composable, allowing easy extension and reuse.

---

## Adding a New Language-Specific Build Template

If you want to add support for a new programming language or build system, please follow these guidelines:

1. **Create a new directory under `workflow-templates/`**
   Name it clearly, e.g. `java-build-template` or `python-build-template`.

2. **Follow the existing composite action pattern**

   * Accept inputs such as `service`, `environment`, and any language-specific options.
   * Receive authentication credentials or environment variables from the `auth` workflows.
   * Perform language-specific build, test, or lint steps.
   * Export any needed outputs to be consumed by downstream workflows.

3. **Integrate with `build-and-publish`**
   Ensure your build outputs align with the expectations of the Docker build and publish workflow. For example, produce the necessary build artifacts or pass build arguments if required.

4. **Security considerations**

   * Do not hardcode secrets; use Vault authentication workflows to inject sensitive data.
   * Validate inputs rigorously and handle errors gracefully.

5. **Testing**

   * Add example workflows or tests to verify your build template works correctly in a typical environment.
   * Test edge cases and failure scenarios.

6. **Documentation**

   * Update this CONTRIBUTING.md if you add new inputs or conventions.
   * Optionally add a README in your template folder describing its usage.

---

## General Contribution Guidelines

* **Branching**
  Create feature branches from `main` and open pull requests.

* **Commits**
  Use clear, descriptive commit messages (e.g., `feat(java): add java build template`).

* **Code style**
  Follow YAML and shell scripting best practices. Ensure proper quoting and error handling.

* **Secrets and tokens**
  Never commit real secrets or tokens. Use repository secrets for sensitive information.

* **Versioning**
  Pin action versions where possible; avoid `@main` in production workflows.

* **Review process**
  All PRs require at least one review and successful workflow runs before merging.

---

## Reporting Issues

Please open issues for bugs, feature requests, or questions. Include:

* Detailed description of the problem or suggestion
* Steps to reproduce if applicable
* Relevant logs or error messages

---

Thank you for helping improve ondgo-products CI/CD workflows! Your effort makes our deployment pipelines robust, secure, and scalable.