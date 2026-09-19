# site-operator

A Kubernetes operator that provisions and manages website deployments (e.g. WordPress) through a single `Site` custom resource — handling the Deployment, Service, Ingress, PVC, database credentials, and an optional file-browser sidecar for you.

## Description

Instead of hand-writing a Deployment, Service, Ingress, PVC, and Secrets for every site you host, define one `Site` custom resource with your domain, database, and storage settings. The operator reconciles the rest of the Kubernetes objects and keeps them in sync with the desired state.

```yaml
apiVersion: site.operator/v1alpha1
kind: Site
metadata:
  name: test-site
  namespace: site-operator
spec:
  domain: example.com
  ingress:
    enabled: true
    path: /
    tls: true
    ingressClassName: nginx
    annotations: {}
  database:
    host: localhost
    name: testdb
    user: testuser
    passwordSecret:
      name: db-password
      key: password
  persistence:
    enabled: true
    size: 5Gi
    storageClassName: standard
```

### Features

- **Ingress management** — optional TLS, custom ingress class, and annotations.
- **Database configuration** — supply credentials directly or reference existing `Secret`s (`userSecret`/`passwordSecret`).
- **Persistence** — provision a new PVC or attach to an existing claim.
- **WordPress support** — optional debug mode and automated admin install (`wordpress.install`).
- **File Browser** — optional sidecar for browsing/managing site files, enabled by default.
- **Status conditions** — standard `Available` / `Progressing` / `Degraded` conditions on every `Site`.

## Getting Started

### Prerequisites
- go version v1.25.3+
- docker version 17.03+
- kubectl version v1.11.3+
- access to a Kubernetes v1.11.3+ cluster

### Run locally against a cluster

```sh
make install   # install the CRDs
make run       # run the controller locally, out-of-cluster
```

### Deploy to a cluster

**Build and push the image:**

```sh
make docker-build docker-push IMG=<some-registry>/site-operator:tag
```

**Install the CRDs:**

```sh
make install
```

**Deploy the manager:**

```sh
make deploy IMG=<some-registry>/site-operator:tag
```

> **NOTE**: If you encounter RBAC errors, make sure you have sufficient cluster permissions (e.g. `cluster-admin`).

**Deploy with Helm instead:**

```sh
helm install site-operator ./helm/site-operator-crds
helm install site-operator ./helm/site-operator
```

### Create a Site

```sh
kubectl apply -f examples/simple.yaml
```

or the sample under `config/samples/`:

```sh
kubectl apply -k config/samples/
```

### Uninstall

```sh
kubectl delete -k config/samples/   # remove Site instances
make uninstall                      # remove CRDs
make undeploy                       # remove the controller
```

## Development

```sh
make manifests   # regenerate CRDs/RBAC from kubebuilder markers
make generate    # regenerate DeepCopy code
make lint-fix    # lint and auto-fix
make test        # run unit tests
make test-e2e    # run e2e tests against a Kind cluster
```

See [AGENTS.md](AGENTS.md) for the project layout and conventions, and run `make help` for the full list of `make` targets.

## Project Distribution

### YAML bundle

```sh
make build-installer IMG=<some-registry>/site-operator:tag
kubectl apply -f https://raw.githubusercontent.com/propastinv/site-operator/<tag or branch>/dist/install.yaml
```

### Helm chart

Charts live under [`helm/site-operator`](helm/site-operator) (controller) and [`helm/site-operator-crds`](helm/site-operator-crds) (CRDs).

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
