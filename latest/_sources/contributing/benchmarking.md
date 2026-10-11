---
title: "Benchmarking"
description: "Measuring inference performance with llm-d-benchmark."
weight: 30
---

# Benchmarking

This directory contains the scripts to run benchmarks on an EKS-based llm-d
deployment using the official
[llm-d-benchmark](https://github.com/llm-d/llm-d-benchmark) tool.


### **2. Set Up Benchmarking Environment**

```bash
# Set your HuggingFace token
export LLMDBENCH_HF_TOKEN=your_huggingface_token

### **3. Run the Benchmark**

./run.sh

### **4. View Results**

```bash
# Check results directory
ls -la ./eks-benchmark-results/

# View analysis plots
open ./eks-benchmark-results/analysis/plots/latency_analysis.png
open ./eks-benchmark-results/analysis/plots/throughput_analysis.png

# View statistics
cat ./eks-benchmark-results/analysis/data/stats.txt
```

## **What You'll Get**

### **Performance Metrics**
- **TTFT (Time to First Token)**: Response latency
- **Throughput**: Tokens per second
- **GPU Utilization**: Resource usage
- **Latency Percentiles**: P50, P95, P99
- **Request Success Rate**: Reliability metrics

### **Visualizations**
- **Latency Analysis**: How latency changes with load
- **Throughput Analysis**: Token processing rates
- **Statistical Summaries**: Detailed performance breakdowns

### **Infrastructure Insights**
- **Resource Utilization**: CPU, memory, GPU usage
- **Scaling Characteristics**: Performance under different loads
- **Bottleneck Identification**: System limitations

## **Configuration Options**

### **Workload Profiles**

- **Light Load**: 0.5-1.0 QPS (queries per second)
- **Medium Load**: 1.0-2.0 QPS
- **Heavy Load**: 2.0-5.0 QPS
- **Test Duration**: 60 seconds per QPS level
- **Concurrent Users**: 5 users
- **Response Length**: 128 tokens

## **Troubleshooting**

### **Common Issues**

**Storage Class Not Found**
   ```bash
   kubectl get storageclass
   ```

## Example AWS EKS Deployment for benchmarking

```bash
kubectl get pods -A
```

```bash
NAMESPACE      NAME                                                        READY   STATUS    RESTARTS      AGE
istio-system   istiod-fcc6f78f4-8867c                                      1/1     Running   0             2d17h
kube-system    aws-load-balancer-controller-6974c7d546-b9k7p               1/1     Running   0             2d17h
kube-system    aws-load-balancer-controller-6974c7d546-jfsrl               1/1     Running   0             2d17h
kube-system    aws-node-lpj7s                                              2/2     Running   0             6m48s
kube-system    aws-node-pfjnh                                              2/2     Running   0             6m51s
kube-system    aws-node-vnlkg                                              2/2     Running   0             6m51s
kube-system    coredns-b59df9565-2xpn8                                     1/1     Running   0             2d17h
kube-system    coredns-b59df9565-sk79z                                     1/1     Running   0             2d17h
kube-system    efs-csi-controller-757d6f8b44-rvsz8                         3/3     Running   0             2d17h
kube-system    efs-csi-controller-757d6f8b44-xsx7x                         3/3     Running   0             2d17h
kube-system    efs-csi-node-2ht8h                                          3/3     Running   0             6m48s
kube-system    efs-csi-node-8mhxc                                          3/3     Running   0             6m51s
kube-system    efs-csi-node-fjn8x                                          3/3     Running   0             6m51s
kube-system    kube-proxy-hj6m9                                            1/1     Running   0             6m51s
kube-system    kube-proxy-r2jnm                                            1/1     Running   0             6m51s
kube-system    kube-proxy-v6nv2                                            1/1     Running   0             6m48s
kube-system    metrics-server-9ffd44bcd-nb6wd                              1/1     Running   0             2d17h
kube-system    metrics-server-9ffd44bcd-s244z                              1/1     Running   0             2d17h
kube-system    nvidia-device-plugin-daemonset-cnd52                        1/1     Running   0             6m34s
kube-system    nvidia-device-plugin-daemonset-g94tv                        1/1     Running   0             6m31s
kube-system    nvidia-device-plugin-daemonset-x9grg                        1/1     Running   0             6m35s
llm-d          llm-d-inference-gateway-istio-666f8c9cb4-q9ktr              1/1     Running   0             2d17h
llm-d          llm-d-modelservice-6879856556-5mxqk                         1/1     Running   0             2d17h
llm-d          llm-d-redis-master-5f77dd4bf9-fnrkh                         1/1     Running   0             2d17h
llm-d          meta-llama-llama-3-2-3b-instruct-decode-54857c5864-67ttp    2/2     Running   3 (40s ago)   2d17h
llm-d          meta-llama-llama-3-2-3b-instruct-epp-6f5556dddd-4qh8r       1/1     Running   0             2d17h
llm-d          meta-llama-llama-3-2-3b-instruct-prefill-678d849849-dlt59   1/1     Running   3 (40s ago)   2d17h
llm-d          meta-llama-llama-3-2-3b-instruct-prefill-678d849849-l84vb   1/1     Running   3 (37s ago)   2d17h
```

```bash
kubectl get pvc -A
```

```bash
NAMESPACE   NAME                    STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   VOLUMEATTRIBUTESCLASS   AGE
efs-test    efs-test-pvc            Bound    pvc-06de80d8-4487-436e-9a8d-320ccbbbbcd5   1Gi        RWX            efs-sc         <unset>                 5d17h
llm-d       benchmark-results-pvc   Bound    pvc-c6fbcd0d-832c-4133-bba0-1cc61f35912c   20Gi       RWX            efs-sc         <unset>                 2d21h
```

```bash
kubectl get svc -A
```

```bash
NAMESPACE      NAME                                                          TYPE           CLUSTER-IP       EXTERNAL-IP                                                                  PORT(S)                                        AGE
default        kubernetes                                                    ClusterIP      10.100.0.1       <none>                                                                       443/TCP                                        9d
istio-system   istiod                                                        ClusterIP      10.100.238.175   <none>                                                                       15010/TCP,15012/TCP,443/TCP,15014/TCP          2d22h
kube-system    aws-load-balancer-webhook-service                             ClusterIP      10.100.218.58    <none>                                                                       443/TCP                                        8d
kube-system    eks-extension-metrics-api                                     ClusterIP      10.100.29.96     <none>                                                                       443/TCP                                        9d
kube-system    kube-dns                                                      ClusterIP      10.100.0.10      <none>                                                                       53/UDP,53/TCP,9153/TCP                         9d
kube-system    metrics-server                                                ClusterIP      10.100.107.163   <none>                                                                       443/TCP                                        9d
kube-system    prometheus-kube-prometheus-coredns                            ClusterIP      None             <none>                                                                       9153/TCP                                       8d
kube-system    prometheus-kube-prometheus-kube-controller-manager            ClusterIP      None             <none>                                                                       10257/TCP                                      8d
kube-system    prometheus-kube-prometheus-kube-etcd                          ClusterIP      None             <none>                                                                       2381/TCP                                       8d
kube-system    prometheus-kube-prometheus-kube-proxy                         ClusterIP      None             <none>                                                                       10249/TCP                                      8d
kube-system    prometheus-kube-prometheus-kube-scheduler                     ClusterIP      None             <none>                                                                       10259/TCP                                      8d
kube-system    prometheus-kube-prometheus-kubelet                            ClusterIP      None             <none>                                                                       10250/TCP,10255/TCP,4194/TCP                   8d
llm-d          llm-d-benchmark-harness                                       ClusterIP      10.100.96.227    <none>                                                                       20873/TCP                                      2d21h
llm-d          llm-d-gateway-loadbalancer                                    LoadBalancer   10.100.4.233     k8s-llmd-llmdgate-c9a7ab0b50-3d0ce93109371f17.elb.eu-north-1.amazonaws.com   80:30577/TCP                                   2d22h
llm-d          llm-d-inference-gateway-istio                                 ClusterIP      10.100.249.195   <none>                                                                       15021/TCP,80/TCP                               2d22h
llm-d          llm-d-modelservice                                            ClusterIP      10.100.83.228    <none>                                                                       8443/TCP                                       2d22h
llm-d          llm-d-redis-headless                                          ClusterIP      None             <none>                                                                       8100/TCP                                       2d22h
llm-d          llm-d-redis-master                                            ClusterIP      10.100.45.218    <none>                                                                       8100/TCP                                       2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-epp-service                  NodePort       10.100.35.216    <none>                                                                       9002:31376/TCP,9003:30216/TCP,9090:31472/TCP   2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-inference-pool-ip-05493744   ClusterIP      None             <none>                                                                       54321/TCP                                      2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-service-decode               ClusterIP      None             <none>                                                                       5557/TCP,8000/TCP                              2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-service-prefill              ClusterIP      None             <none>                                                                       5557/TCP,8000/TCP                              2d22h
```

```bash
kubectl get endpoints -A
```

```bash
NAMESPACE      NAME                                                          ENDPOINTS                                                                    AGE
default        kubernetes                                                    192.168.126.125:443,192.168.77.119:443                                       9d
istio-system   istiod                                                        192.168.56.51:15012,192.168.56.51:15010,192.168.56.51:15017 + 1 more...      2d22h
kube-system    aws-load-balancer-webhook-service                             192.168.32.110:9443,192.168.41.197:9443                                      8d
kube-system    eks-extension-metrics-api                                     172.0.32.0:10443                                                             9d
kube-system    kube-dns                                                      192.168.46.69:53,192.168.51.199:53,192.168.46.69:53 + 3 more...              9d
kube-system    metrics-server                                                192.168.42.232:10251,192.168.57.254:10251                                    9d
kube-system    prometheus-kube-prometheus-coredns                            192.168.46.69:9153,192.168.51.199:9153                                       8d
kube-system    prometheus-kube-prometheus-kube-controller-manager            <none>                                                                       8d
kube-system    prometheus-kube-prometheus-kube-etcd                          <none>                                                                       8d
kube-system    prometheus-kube-prometheus-kube-proxy                         192.168.37.57:10249,192.168.4.150:10249,192.168.41.29:10249                  8d
kube-system    prometheus-kube-prometheus-kube-scheduler                     <none>                                                                       8d
kube-system    prometheus-kube-prometheus-kubelet                            192.168.23.188:10250,192.168.44.175:10250,192.168.48.149:10250 + 6 more...   8d
llm-d          llm-d-benchmark-harness                                       <none>                                                                       2d21h
llm-d          llm-d-gateway-loadbalancer                                    192.168.35.232:80                                                            2d22h
llm-d          llm-d-inference-gateway-istio                                 192.168.35.232:80,192.168.35.232:15021                                       2d22h
llm-d          llm-d-modelservice                                            192.168.57.2:8443                                                            2d22h
llm-d          llm-d-redis-headless                                          192.168.44.194:6379                                                          2d22h
llm-d          llm-d-redis-master                                            192.168.44.194:6379                                                          2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-epp-service                  192.168.62.241:9003,192.168.62.241:9090,192.168.62.241:9002                  2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-inference-pool-ip-05493744                                                                                2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-service-decode                                                                                            2d22h
llm-d          meta-llama-llama-3-2-3b-instruct-service-prefill                                                                                           2d22h
```
