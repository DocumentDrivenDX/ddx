---
source: https://12factor.net/
---

# The Twelve-Factor App

## Introduction

The twelve-factor app is a methodology for building software-as-a-service applications that:

- Use **declarative** formats for setup automation, to minimize time and cost for new developers joining the project
- Have a **clean contract** with the underlying operating system, offering maximum portability between execution environments
- Are suitable for **deployment** on modern cloud platforms, obviating the need for servers and systems administration
- **Minimize divergence** between development and production, enabling continuous deployment for maximum agility
- And can **scale up** without significant changes to tooling, architecture, or development practices

The twelve-factor methodology can be applied to apps written in any programming language, and which use any combination of backing services (database, queue, memory cache, etc).

## Who should read this document?

Any developer building applications which run as a service. Ops engineers who deploy or manage such applications.

## The Twelve Factors

### I. Codebase
**One codebase tracked in revision control, many deploys**

There is always a one-to-one correlation between the codebase and the app. Multiple apps sharing the same code is a violation of twelve-factor. The solution here is to factor shared code into libraries which can be included through the dependency manager. One codebase maps to many deploys across different environments (development, staging, production).

### II. Dependencies
**Explicitly declare and isolate dependencies**

A twelve-factor app never relies on implicit existence of system-wide packages. It declares all dependencies, completely and exactly, via a dependency declaration manifest. Furthermore, it uses a dependency isolation tool during execution to ensure that no implicit dependencies "leak in" from the surrounding system. The full and explicit dependency specification is applied uniformly to both production and development.

### III. Config
**Store config in the environment**

An app's config is everything that is likely to vary between deploys (staging, production, developer environments, etc). This includes database handles, credentials for external services, and per-deploy values such as the canonical hostname for the deploy. Apps sometimes store config as constants in the code. This is a violation of twelve-factor, which requires strict separation of config from code. Config varies substantially across deploys, code does not.

### IV. Backing services
**Treat backing services as attached resources**

A backing service is any service the app consumes over the network as part of its normal operation. Examples include datastores, messaging/queueing systems, SMTP services for outbound email, and caching systems. The code for a twelve-factor app makes no distinction between local and third party services. To the app, both are attached resources, accessed via a URL or other locator/credentials stored in the config.

### V. Build, release, run
**Strictly separate build and run stages**

A codebase is transformed into a (non-development) deploy through three stages: The build stage converts code repo into an executable bundle known as a build. The release stage takes the build and combines it with the deploy's current config to create a release. The run stage runs the app in the execution environment by launching some set of the app's processes against a selected release. The twelve-factor app uses strict separation between these stages.

### VI. Processes
**Execute the app as one or more stateless processes**

The app is executed in the execution environment as one or more processes. Twelve-factor processes are stateless and share-nothing. Any data that needs to persist must be stored in a stateful backing service, typically a database. The memory space or filesystem of the process can be used as a brief, single-transaction cache. Sticky sessions are a violation of twelve-factor and should never be used or relied upon.

### VII. Port binding
**Export services via port binding**

Web apps are sometimes executed inside a webserver container. The twelve-factor app is completely self-contained and does not rely on runtime injection of a webserver into the execution environment to create a web-facing service. The web app exports HTTP as a service by binding to a port, and listening to requests coming in on that port. In deployment, a routing layer handles routing requests from a public-facing hostname to the port-bound web processes.

### VIII. Concurrency
**Scale out via the process model**

In the twelve-factor app, processes are a first class citizen. Processes in the twelve-factor app take strong cues from the unix process model for running service daemons. Using this model, the developer can architect their app to handle diverse workloads by assigning each type of work to a process type. The app can span multiple processes running on multiple physical machines.

### IX. Disposability
**Maximize robustness with fast startup and graceful shutdown**

The twelve-factor app's processes are disposable, meaning they can be started or stopped at a moment's notice. This facilitates fast elastic scaling, rapid deployment of code or config changes, and robustness of production deploys. Processes should strive to minimize startup time and shut down gracefully when they receive a SIGTERM signal. Processes should also be robust against sudden death.

### X. Dev/prod parity
**Keep development, staging, and production as similar as possible**

The twelve-factor app is designed for continuous deployment by keeping the gap between development and production small. The traditional gaps include: the time gap (code takes days or weeks to go to production), the personnel gap (developers write code, ops engineers deploy it), and the tools gap (different stack in development vs production). The twelve-factor developer resists the urge to use different backing services between development and production.

### XI. Logs
**Treat logs as event streams**

A twelve-factor app never concerns itself with routing or storage of its output stream. It should not attempt to write to or manage logfiles. Instead, each running process writes its event stream, unbuffered, to stdout. In staging or production deploys, each process' stream will be captured by the execution environment, collated together with all other streams from the app, and routed to one or more final destinations for viewing and long-term archival.

### XII. Admin processes
**Run admin/management tasks as one-off processes**

One-off admin processes should be run in an identical environment as the regular long-running processes of the app. They run against a release, using the same codebase and config as any process run against that release. Admin code must ship with application code to avoid synchronization issues. The same dependency isolation techniques should be used on all process types.