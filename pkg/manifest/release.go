package manifest

var release = Release{
	PreviousVersion:   "0.0.0",
	KubernetesVersion: "1.15.5",
	DockerVersion:     "19.03.1",
	EtcdVersion:       "v3.4.1",
	Dependencies: Dependencies{
		ControlPlane: controlplane,
		Core:         core,
	},
}

var controlplane = map[string]Dependency{
	"haproxy": Dependency{
		Version:      "1.9.4",
		Name:         "haproxy",
		Src:          "docker.io/aloha2you/haproxy:1.9.4-alpine",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/docker.io/aloha2you/haproxy-1.9.4.tar.xz",
		Checksum:     "19008fcacf3904f32856d074779090c6cd7e9869d1b37e4c5c06a6f54dc02c30",
		ChecksumType: "sha256",
		LicenseURL:   "",
	},
	"keepalived": Dependency{
		Version:      "2.0.11",
		Name:         "keepalived",
		Src:          "docker.io/aloha2you/keepalived:2.0.11",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/docker.io/aloha2you/keepalived-2.0.11.tar.xz",
		Checksum:     "315846bd98de7edfa339e398143aedf640f0826e6c1d89cfb6665a962483f167",
		ChecksumType: "sha256",
		LicenseURL:   "",
	},
	"etcd": Dependency{
		Version:      "v3.4.1",
		Name:         "etcd",
		Src:          "gcr.io/etcd-development/etcd:v3.4.1",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/etcd-development/etcd-v3.4.1.tar.xz",
		Checksum:     "44baa910d80769b3c9799adb29ef2b1198c3aa9d63edaa6e618ed8f24600d11e",
		ChecksumType: "sha256",
		LicenseURL:   "https://github.com/etcd-io/etcd/blob/master/LICENSE",
	},
	"kube-apiserver": Dependency{
		Version:      "v1.15.3-0",
		Name:         "kube-apiserver",
		Src:          "gcr.io/google-containers/hyperkube:v1.15.5",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/google-containers/hyperkube-v1.15.5.tar.xz",
		Checksum:     "c0a2b93f543177574551262ba1cf9f005fa4d2a046c731070f1d39f829d50e5f",
		ChecksumType: "sha256",
	},
	"kube-controller-manager": Dependency{
		Version:      "v1.15.3-0",
		Name:         "kube-controller-manager",
		Src:          "gcr.io/google-containers/hyperkube:v1.15.5",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/google-containers/hyperkube-v1.15.5.tar.xz",
		Checksum:     "c0a2b93f543177574551262ba1cf9f005fa4d2a046c731070f1d39f829d50e5f",
		ChecksumType: "sha256",
	},
	"kube-scheduler": Dependency{
		Version:      "v1.15.3-0",
		Name:         "kube-scheduler",
		Src:          "gcr.io/google-containers/hyperkube:v1.15.5",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/google-containers/hyperkube-v1.15.5.tar.xz",
		Checksum:     "c0a2b93f543177574551262ba1cf9f005fa4d2a046c731070f1d39f829d50e5f",
		ChecksumType: "sha256",
	},
	"kube-proxy": Dependency{
		Version:      "v1.15.3-0",
		Name:         "kube-proxy",
		Src:          "gcr.io/google-containers/hyperkube:v1.15.5",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/google-containers/hyperkube-v1.15.5.tar.xz",
		Checksum:     "c0a2b93f543177574551262ba1cf9f005fa4d2a046c731070f1d39f829d50e5f",
		ChecksumType: "sha256",
	},
	"kubelet": Dependency{
		Version:      "v1.15.3-0",
		Name:         "kubelet",
		Src:          "gcr.io/google-containers/hyperkube:v1.15.5",
		PrebakePath:  "/opt/liferaft/kubekit-control-plane/gcr.io/google-containers/hyperkube-v1.15.5.tar.xz",
		Checksum:     "c0a2b93f543177574551262ba1cf9f005fa4d2a046c731070f1d39f829d50e5f",
		ChecksumType: "sha256",
	},
	"pause": Dependency{
		Version:      "3.1",
		Name:         "pause",
		Src:          "k8s.gcr.io/pause:3.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/k8s.gcr.io/pause-3.1.tar.xz",
		Checksum:     "f78411e19d84a252e53bff71a4407a5686c46983a2c2eeed83929b888179acea",
		ChecksumType: "sha256",
	},
}

var core = map[string]Dependency{
	"addon-resizer": Dependency{
		Version:      "1.8.3",
		Name:         "addon-resizer",
		Src:          "gcr.io/google_containers/addon-resizer:1.8.3",
		PrebakePath:  "/opt/liferaft/kubekit-core/gcr.io/google_containers/addon-resizer-1.8.3.tar.xz",
		Checksum:     "07353f7b26327f0d933515a22b1de587b040d3d85c464ea299c1b9f242529326",
		ChecksumType: "sha256",
	},
	"ingress-controller": Dependency{
		Version:      "0.9.0-beta.15",
		Name:         "nginx-ingress-controller",
		Src:          "gcr.io/google_containers/nginx-ingress-controller:0.9.0-beta.15",
		PrebakePath:  "/opt/liferaft/kubekit-core/gcr.io/google_containers/nginx-ingress-controller-0.9.0-beta.15.tar.xz",
		Checksum:     "1c64bc6dfb7ddbe4a0a9fce7f5c521aa13e7836c3b90601897b763add8494a41",
		ChecksumType: "sha256",
	},
	"default-backend": Dependency{
		Version:      "1.0",
		Name:         "default-backend",
		Src:          "gcr.io/google_containers/defaultbackend:1.0",
		PrebakePath:  "/opt/liferaft/kubekit-core/gcr.io/google_containers/defaultbackend-1.0.tar.xz",
		Checksum:     "ee3aa1187023d0197e3277833f19d9ef7df26cee805fef32663e06c7412239f9",
		ChecksumType: "sha256",
	},
	"calico-node": Dependency{
		Version:      "v3.5.1",
		Name:         "calico-node",
		Src:          "quay.io/calico/node:v3.5.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/quay.io/calico/node-v3.5.1.tar.xz",
		Checksum:     "3d6451fd33d4ca6396a8dd2ef29a9e90fef1bd4a67ba6d5cc89d7f9e33d7d55f",
		ChecksumType: "sha256",
	},
	"calico-cni": Dependency{
		Version:      "v3.5.1",
		Name:         "calico-cni",
		Src:          "quay.io/calico/cni:v3.5.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/quay.io/calico/cni-v3.5.1.tar.xz",
		Checksum:     "a40b7daa307d0dbc5f3528143eb19c0ece133f3af0da72bc233d1feb6bc5770a",
		ChecksumType: "sha256",
	},
	"calico-typha": Dependency{
		Version:      "v3.5.1",
		Name:         "calico-typha",
		Src:          "quay.io/calico/typha:v3.5.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/quay.io/calico/typha-v3.5.1.tar.xz",
		Checksum:     "7f47e3317e4f2d167e1a3511b13adb0fd4f17f554a0ba285db2e8dc5aefcb794",
		ChecksumType: "sha256",
	},
	"cluster-proportional-autoscaler": Dependency{
		Version:      "1.3.0",
		Name:         "cluster-proportional-autoscaler",
		Src:          "k8s.gcr.io/cluster-proportional-autoscaler-amd64:1.3.0",
		PrebakePath:  "/opt/liferaft/kubekit-core/k8s.gcr.io/cluster-proportional-autoscaler-amd64-1.3.0.tar.xz",
		Checksum:     "4fd37c5b29a38b02c408c56254bd1a3a76f3e236610bc7a8382500bbf9ecfc76",
		ChecksumType: "sha256",
	},
	"coredns": Dependency{
		Version:      "1.5.0",
		Name:         "coredns",
		Src:          "docker.io/coredns/coredns:1.5.0",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/coredns/coredns:1.5.0.tar.xz",
		Checksum:     "e83beb5e43f8513fa735e77ffc5859640baea30a882a11cc75c4c3244a737d3c",
		ChecksumType: "sha256",
	},
	"dns-aaaa-delay": Dependency{
		Version:      "v0.0.1",
		Name:         "dns-aaaa-delay",
		Src:          "docker.io/aloha2you/giantswarm-dns-aaaa-delay:v0.0.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/aloha2you/giantswarm-dns-aaaa-delay-v0.0.1.tar.xz",
		Checksum:     "d110046611b75b6da2fca5f3cbf4ba578c8c67111d3a1b95224d1a0fd9a8aa75",
		ChecksumType: "sha256",
	},
	"registry": Dependency{
		Version:      "v2.6.2",
		Name:         "docker-registry",
		Src:          "docker.io/registry:2.6.2",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/registry-2.6.2.tar.xz",
		Checksum:     "5a156ff125e5a12ac7fdec2b90b7e2ae5120fa249cf62248337b6d04abc574c8",
		ChecksumType: "sha256",
	},
	"heapster": Dependency{
		Version:      "v1.5.4",
		Name:         "heapster",
		Src:          "k8s.gcr.io/heapster-amd64:v1.5.4",
		PrebakePath:  "/opt/liferaft/kubekit-core/k8s.gcr.io/heapster-amd64-v1.5.4.tar.xz",
		Checksum:     "dccaabb0c20cf05c29baefa1e9bf0358b083ccc0fab492b9b3b47fb7e4db5472",
		ChecksumType: "sha256",
	},
	"kubernetes-dashboard": Dependency{
		Version:      "v1.10.1",
		Name:         "kubernetes-dashboard",
		Src:          "k8s.gcr.io/kubernetes-dashboard-amd64:v1.10.1",
		PrebakePath:  "/opt/liferaft/kubekit-core/k8s.gcr.io/kubernetes-dashboard-amd64-v1.10.1.tar.xz",
		Checksum:     "0ae6b69432e78069c5ce2bcde0fe409c5c4d6f0f4d9cd50a17974fea38898747",
		ChecksumType: "sha256",
	},
	"kube-state-metrics": Dependency{
		Version:      "v1.4.0",
		Name:         "kube-state-metrics",
		Src:          "gcr.io/google_containers/kube-state-metrics:v1.4.0",
		PrebakePath:  "/opt/liferaft/kubekit-core/gcr.io/google_containers/kube-state-metrics-v1.4.0.tar.xz",
		Checksum:     "49b0d96d872c3c85959d57bcb9bc4e661fda9e66991490b2ec738464396a4010",
		ChecksumType: "sha256",
	},
	"rook-ceph": Dependency{
		Version:      "v1.0.6",
		Name:         "rook-ceph",
		Src:          "docker.io/rook/ceph:v1.0.6",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/rook/ceph-v1.0.6.tar.xz",
		Checksum:     "c8b48548e439edaa4958bd24e1e426fca7d79e76a752e2291b9630b95008e438",
		ChecksumType: "sha256",
	},
	"ceph": Dependency{
		Version:      "v14.2.2-20190722",
		Name:         "ceph",
		Src:          "docker.io/ceph/ceph:v14.2.2-20190722",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/ceph/ceph-v14.2.2-20190722.tar.xz",
		Checksum:     "43d62cfba07ef79b66068c53346be8e6fe2d21cf22c7ac3cdd967a188b4d5c7f",
		ChecksumType: "sha256",
	},
	"open-policy-agent": Dependency{
		Version:      "0.11.0",
		Name:         "opa",
		Src:          "docker.io/openpolicyagent/opa:0.11.0",
		PrebakePath:  "/opt/liferaft/kubekit-core/docker.io/openpolicyagent/opa-0.11.0.tar.xz",
		Checksum:     "9d5e59018983cfd4dc3fdd1e181e52f6dcdcb64d7652653f579ebe14734247f9",
		ChecksumType: "sha256",
	},
}
