package commands

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/docker/machine/cli"
	"github.com/kubernetes/helm/pkg/kubectl"
)

func generateK8sURL(url string) string {
	mParts := strings.Split(url, "://")
	mParts = strings.Split(mParts[1], ":")

	k8sHost := fmt.Sprintf("https://%s:6443", mParts[0])

	return k8sHost
}

func cmdDeploy(c *cli.Context) error {
	if len(c.Args()) != 2 {
		return fmt.Errorf("Requires a machine and deployment file location")
	}

	host, err := getFirstArgHost(c)
	if err != nil {
		return err
	}

	t := c.Args().Get(1)

	/* type for now can be one of: dns|helm|dashboard */
	var txt string
	switch t {
	case "dns":
		txt = `
######################################################################
# Copyright 2015 The Kubernetes Authors All rights reserved.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
######################################################################
---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    name: kube-system
  name: kube-system
---
apiVersion: v1
kind: Service
metadata:
  name: kube-dns
  namespace: kube-system
  labels:
    k8s-app: kube-dns
    kubernetes.io/cluster-service: "true"
    kubernetes.io/name: "KubeDNS"
spec:
  selector:
    k8s-app: kube-dns
  clusterIP:  10.0.0.10
  ports:
  - name: dns
    port: 53
    protocol: UDP
  - name: dns-tcp
    port: 53
    protocol: TCP
---
apiVersion: v1
kind: ReplicationController
metadata:
  name: kube-dns-v11
  namespace: kube-system
  labels:
    k8s-app: kube-dns
    version: v11
    kubernetes.io/cluster-service: "true"
spec:
  replicas: 1
  selector:
    k8s-app: kube-dns
    version: v11
  template:
    metadata:
      labels:
        k8s-app: kube-dns
        version: v11
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: etcd
        image: gcr.io/google_containers/etcd-amd64:2.2.1
        resources:
          # TODO: Set memory limits when we've profiled the container for large
          # clusters, then set request = limit to keep this container in
          # guaranteed class. Currently, this container falls into the
          # "burstable" category so the kubelet doesn't backoff from restarting it.
          limits:
            cpu: 100m
            memory: 500Mi
          requests:
            cpu: 100m
            memory: 50Mi
        command:
        - /usr/local/bin/etcd
        - -data-dir
        - /var/etcd/data
        - -listen-client-urls
        - http://127.0.0.1:2379,http://127.0.0.1:4001
        - -advertise-client-urls
        - http://127.0.0.1:2379,http://127.0.0.1:4001
        - -initial-cluster-token
        - skydns-etcd
        volumeMounts:
        - name: etcd-storage
          mountPath: /var/etcd/data
      - name: kube2sky
        image: gcr.io/google_containers/kube2sky:1.14
        resources:
          # TODO: Set memory limits when we've profiled the container for large
          # clusters, then set request = limit to keep this container in
          # guaranteed class. Currently, this container falls into the
          # "burstable" category so the kubelet doesn't backoff from restarting it.
          limits:
            cpu: 100m
            # Kube2sky watches all pods.
            memory: 200Mi
          requests:
            cpu: 100m
            memory: 50Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 60
          timeoutSeconds: 5
          successThreshold: 1
          failureThreshold: 5
        readinessProbe:
          httpGet:
            path: /readiness
            port: 8081
            scheme: HTTP
          # we poll on pod startup for the Kubernetes master service and
          # only setup the /readiness HTTP server once that's available.
          initialDelaySeconds: 30
          timeoutSeconds: 5
        args:
        # command = "/kube2sky"
        - --domain=cluster.local
      - name: skydns
        image: gcr.io/google_containers/skydns:2015-10-13-8c72f8c
        resources:
          # TODO: Set memory limits when we've profiled the container for large
          # clusters, then set request = limit to keep this container in
          # guaranteed class. Currently, this container falls into the
          # "burstable" category so the kubelet doesn't backoff from restarting it.
          limits:
            cpu: 100m
            memory: 200Mi
          requests:
            cpu: 100m
            memory: 50Mi
        args:
        # command = "/skydns"
        - -machines=http://127.0.0.1:4001
        - -addr=0.0.0.0:53
        - -ns-rotate=false
        - -domain=cluster.local.
        ports:
        - containerPort: 53
          name: dns
          protocol: UDP
        - containerPort: 53
          name: dns-tcp
          protocol: TCP
      - name: healthz
        image: gcr.io/google_containers/exechealthz:1.0
        resources:
          # keep request = limit to keep this container in guaranteed class
          limits:
            cpu: 10m
            memory: 20Mi
          requests:
            cpu: 10m
            memory: 20Mi
        args:
        - -cmd=nslookup kubernetes.default.svc.cluster.local 127.0.0.1 >/dev/null
        - -port=8080
        ports:
        - containerPort: 8080
          protocol: TCP
      volumes:
      - name: etcd-storage
        emptyDir: {}
      dnsPolicy: Default  # Don't use cluster DNS.`
	case "helm":
		txt = `
######################################################################
# Copyright 2015 The Kubernetes Authors All rights reserved.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
######################################################################
---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    app: helm
    name: helm-namespace
  name: helm
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: helm
    name: expandybird-service
  name: expandybird-service
  namespace: helm
spec:
  ports:
  - name: expandybird
    port: 8081
    targetPort: 8080
  selector:
    app: helm
    name: expandybird
---
apiVersion: v1
kind: ReplicationController
metadata:
  labels:
    app: helm
    name: expandybird-rc
  name: expandybird-rc
  namespace: helm
spec:
  replicas: 2
  selector:
    app: helm
    name: expandybird
  template:
    metadata:
      labels:
        app: helm
        name: expandybird
    spec:
      containers:
      - env: []
        image: gcr.io/dm-k8s-prod/expandybird:v1.2.1
        name: expandybird
        ports:
        - containerPort: 8080
          name: expandybird
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: helm
    name: resourcifier-service
  name: resourcifier-service
  namespace: helm
spec:
  ports:
  - name: resourcifier
    port: 8082
    targetPort: 8080
  selector:
    app: helm
    name: resourcifier
---
apiVersion: v1
kind: ReplicationController
metadata:
  labels:
    app: helm
    name: resourcifier-rc
  name: resourcifier-rc
  namespace: helm
spec:
  replicas: 2
  selector:
    app: helm
    name: resourcifier
  template:
    metadata:
      labels:
        app: helm
        name: resourcifier
    spec:
      containers:
      - env: []
        image: gcr.io/dm-k8s-prod/resourcifier:v1.2.1
        name: resourcifier
        ports:
        - containerPort: 8080
          name: resourcifier
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app: helm
    name: manager-service
  name: manager-service
  namespace: helm
spec:
  ports:
  - name: manager
    port: 8080
    targetPort: 8080
  selector:
    app: helm
    name: manager
---
apiVersion: v1
kind: ReplicationController
metadata:
  labels:
    app: helm
    name: manager-rc
  name: manager-rc
  namespace: helm
spec:
  replicas: 1
  selector:
    app: helm
    name: manager
  template:
    metadata:
      labels:
        app: helm
        name: manager
    spec:
      containers:
      - env: []
        image: gcr.io/dm-k8s-prod/manager:v1.2.1
        name: manager
        ports:
        - containerPort: 8080
          name: manager`
	case "dashboard":
		txt = `
######################################################################
# Copyright 2015 The Kubernetes Authors All rights reserved.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
######################################################################
---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    name: kube-system
  name: kube-system
---
apiVersion: v1
kind: ReplicationController
metadata:
  # Keep the name in sync with image version and
  # gce/coreos/kube-manifests/addons/dashboard counterparts
  name: kubernetes-dashboard-v1.0.1
  namespace: kube-system
  labels:
    k8s-app: kubernetes-dashboard
    version: v1.0.1
    kubernetes.io/cluster-service: "true"
spec:
  replicas: 1
  selector:
    k8s-app: kubernetes-dashboard
  template:
    metadata:
      labels:
        k8s-app: kubernetes-dashboard
        version: v1.0.1
        kubernetes.io/cluster-service: "true"
    spec:
      containers:
      - name: kubernetes-dashboard
        image: gcr.io/google_containers/kubernetes-dashboard-amd64:v1.0.1
        resources:
          # keep request = limit to keep this container in guaranteed class
          limits:
            cpu: 100m
            memory: 50Mi
          requests:
            cpu: 100m
            memory: 50Mi
        ports:
        - containerPort: 9090
        livenessProbe:
          httpGet:
            path: /
            port: 9090
          initialDelaySeconds: 30
          timeoutSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: kubernetes-dashboard
  namespace: kube-system
  labels:
    k8s-app: kubernetes-dashboard
    kubernetes.io/cluster-service: "true"
spec:
  selector:
    k8s-app: kubernetes-dashboard
  ports:
  - port: 80
    targetPort: 9090
`
	default:
		return fmt.Errorf("Invalid file: %s", t)
	}

	buf := bytes.NewBufferString(txt)

	url, err := host.Driver.GetURL()
	if err != nil {
		return err
	}

	fmt.Println("using host: " + generateK8sURL(url))
	runner := &kubectl.RealRunner{}

	runner.Create(buf.Bytes())

	return nil
}
