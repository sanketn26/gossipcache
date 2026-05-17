# GossipCache Deployment Guide

## Overview

This guide covers deploying GossipCache across different environments: EC2 instances, Docker containers, and Kubernetes clusters. Each environment has unique considerations for node discovery, networking, and configuration.

## Table of Contents

1. [EC2 Deployment](#ec2-deployment)
2. [Docker Deployment](#docker-deployment)
3. [Kubernetes Deployment](#kubernetes-deployment)
4. [Configuration Reference](#configuration-reference)
5. [Monitoring & Operations](#monitoring--operations)
6. [Troubleshooting](#troubleshooting)

---

## EC2 Deployment

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        AWS VPC                               │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Application Tier (Auto Scaling)           │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐            │ │
│  │  │  EC2 +   │  │  EC2 +   │  │  EC2 +   │            │ │
│  │  │  App +   │  │  App +   │  │  App +   │            │ │
│  │  │  Cache   │  │  Cache   │  │  Cache   │            │ │
│  │  └────┬─────┘  └────┬─────┘  └────┬─────┘            │ │
│  │       │             │             │                    │ │
│  │       └─────────────┼─────────────┘                    │ │
│  │                     │                                  │ │
│  │             Gossip Protocol (TCP/UDP)                  │ │
│  └────────────────────────────────────────────────────────┘ │
│                          │                                   │
│                          │                                   │
│  ┌────────────────────────────────────────────────────────┐ │
│  │              Data Tier (Backed Mode)                   │ │
│  │  ┌──────────────────────────────────────────────────┐ │ │
│  │  │   ElastiCache Redis / RDS Postgres               │ │ │
│  │  └──────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Prerequisites

- AWS account with EC2 permissions
- VPC with private subnets
- Security groups configured for gossip ports
- IAM role with EC2 describe permissions (for discovery)
- Optional: ElastiCache Redis or RDS (for backed mode)

### Node Discovery via EC2 Tags

GossipCache nodes discover each other using EC2 instance tags.

**Required IAM Permissions:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeInstances",
        "ec2:DescribeTags"
      ],
      "Resource": "*"
    }
  ]
}
```

**Tag-Based Discovery Configuration:**

```yaml
# config.yaml
discovery:
  mode: ec2
  ec2:
    region: us-east-1
    tag_key: gossipcache-cluster
    tag_value: production
    refresh_interval: 30s
```

### Security Group Configuration

**Inbound Rules:**

| Type | Protocol | Port Range | Source | Description |
|------|----------|------------|--------|-------------|
| Custom TCP | TCP | 7946 | sg-gossipcache | Gossip TCP |
| Custom UDP | UDP | 7946 | sg-gossipcache | Gossip UDP |
| Custom TCP | TCP | 8080 | sg-app-lb | HTTP API (optional) |
| Custom TCP | TCP | 9090 | sg-monitoring | Metrics |

### Deployment Steps

#### 1. Launch EC2 Instances

```bash
# Launch via AWS CLI
aws ec2 run-instances \
  --image-id ami-xxxxx \
  --instance-type t3.medium \
  --key-name my-key \
  --security-group-ids sg-gossipcache \
  --subnet-id subnet-xxxxx \
  --iam-instance-profile Name=gossipcache-role \
  --tag-specifications 'ResourceType=instance,Tags=[{Key=gossipcache-cluster,Value=production}]' \
  --user-data file://install-gossipcache.sh \
  --count 3
```

#### 2. User Data Script

```bash
#!/bin/bash
# install-gossipcache.sh

# Install GossipCache
wget https://github.com/sanketn26/gossipcache/releases/download/v0.1.0/gossipcache-linux-amd64
chmod +x gossipcache-linux-amd64
mv gossipcache-linux-amd64 /usr/local/bin/gossipcache

# Get instance metadata
INSTANCE_ID=$(ec2-metadata --instance-id | cut -d " " -f 2)
PRIVATE_IP=$(ec2-metadata --local-ipv4 | cut -d " " -f 2)
REGION=$(ec2-metadata --availability-zone | cut -d " " -f 2 | sed 's/.$//')

# Create config
cat > /etc/gossipcache/config.yaml <<EOF
mode: backed
node_id: ${INSTANCE_ID}
address: ${PRIVATE_IP}:7946

cache:
  max_size: 2GB
  default_ttl: 5m
  eviction_policy: lru

gossip:
  interval: 1s
  fanout: 3
  anti_entropy_interval: 5m

backing_store:
  type: redis
  address: ${REDIS_ENDPOINT}:6379

discovery:
  mode: ec2
  ec2:
    region: ${REGION}
    tag_key: gossipcache-cluster
    tag_value: production

network:
  tcp_port: 7946
  udp_port: 7946

metrics:
  enabled: true
  port: 9090
EOF

# Start service
systemctl start gossipcache
systemctl enable gossipcache
```

#### 3. Auto Scaling Group (Optional)

```bash
# Create launch template
aws ec2 create-launch-template \
  --launch-template-name gossipcache \
  --version-description "v1" \
  --launch-template-data file://launch-template.json

# Create auto scaling group
aws autoscaling create-auto-scaling-group \
  --auto-scaling-group-name gossipcache-asg \
  --launch-template LaunchTemplateName=gossipcache,Version='$Latest' \
  --min-size 3 \
  --max-size 10 \
  --desired-capacity 5 \
  --vpc-zone-identifier "subnet-a,subnet-b,subnet-c" \
  --health-check-type EC2 \
  --health-check-grace-period 300 \
  --tags Key=gossipcache-cluster,Value=production
```

### Backed Mode with ElastiCache Redis

```yaml
# config.yaml (backed mode)
mode: backed

backing_store:
  type: redis
  address: gossipcache-prod.xxxxx.cache.amazonaws.com:6379
  pool_size: 50
  timeout: 500ms

gossip:
  interval: 1s  # Gossip metadata only
```

### Independent Mode (No Backing Store)

```yaml
# config.yaml (independent mode)
mode: independent

conflict_resolution:
  strategy: last_write_wins

gossip:
  interval: 500ms  # More frequent, carries full data
```

---

## Docker Deployment

### Architecture

```
┌───────────────────────────────────────────────────────┐
│                   Docker Host                          │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐            │
│  │ App      │  │ App      │  │ App      │            │
│  │Container │  │Container │  │Container │            │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
│       │             │             │                    │
│       │   Docker Bridge Network   │                    │
│       │             │             │                    │
│  ┌────▼─────┐  ┌───▼──────┐  ┌──▼───────┐            │
│  │GossipCache│ │GossipCache│ │GossipCache│           │
│  │Container  │ │Container  │ │Container  │           │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘            │
│       │             │             │                    │
│       └─────────────┼─────────────┘                    │
│                     │                                  │
│                Gossip Protocol                         │
│                                                         │
│  ┌──────────────────────────────────────────────────┐ │
│  │              Redis Container                      │ │
│  │              (Backed Mode)                        │ │
│  └──────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────┘
```

### Docker Compose Setup

#### Backed Mode with Redis

```yaml
# docker-compose.yml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - gossipcache-net
    volumes:
      - redis-data:/data

  cache1:
    image: gossipcache:latest
    container_name: gossipcache-1
    environment:
      - NODE_ID=cache1
      - MODE=backed
      - BACKING_STORE_TYPE=redis
      - BACKING_STORE_ADDRESS=redis:6379
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache2:7946,cache3:7946
    ports:
      - "7946:7946"
      - "7946:7946/udp"
      - "9090:9090"
    networks:
      - gossipcache-net
    depends_on:
      - redis

  cache2:
    image: gossipcache:latest
    container_name: gossipcache-2
    environment:
      - NODE_ID=cache2
      - MODE=backed
      - BACKING_STORE_TYPE=redis
      - BACKING_STORE_ADDRESS=redis:6379
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache1:7946,cache3:7946
    ports:
      - "7947:7946"
      - "7947:7946/udp"
      - "9091:9090"
    networks:
      - gossipcache-net
    depends_on:
      - redis

  cache3:
    image: gossipcache:latest
    container_name: gossipcache-3
    environment:
      - NODE_ID=cache3
      - MODE=backed
      - BACKING_STORE_TYPE=redis
      - BACKING_STORE_ADDRESS=redis:6379
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache1:7946,cache2:7946
    ports:
      - "7948:7946"
      - "7948:7946/udp"
      - "9092:9090"
    networks:
      - gossipcache-net
    depends_on:
      - redis

networks:
  gossipcache-net:
    driver: bridge

volumes:
  redis-data:
```

#### Independent Mode (No Redis)

```yaml
# docker-compose-independent.yml
version: '3.8'

services:
  cache1:
    image: gossipcache:latest
    container_name: gossipcache-1
    environment:
      - NODE_ID=cache1
      - MODE=independent
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache2:7946,cache3:7946
      - CONFLICT_STRATEGY=last_write_wins
    ports:
      - "7946:7946"
      - "7946:7946/udp"
    networks:
      - gossipcache-net

  cache2:
    image: gossipcache:latest
    container_name: gossipcache-2
    environment:
      - NODE_ID=cache2
      - MODE=independent
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache1:7946,cache3:7946
      - CONFLICT_STRATEGY=last_write_wins
    ports:
      - "7947:7946"
      - "7947:7946/udp"
    networks:
      - gossipcache-net

  cache3:
    image: gossipcache:latest
    container_name: gossipcache-3
    environment:
      - NODE_ID=cache3
      - MODE=independent
      - DISCOVERY_MODE=docker
      - GOSSIP_PEERS=cache1:7946,cache2:7946
      - CONFLICT_STRATEGY=last_write_wins
    ports:
      - "7948:7946"
      - "7948:7946/udp"
    networks:
      - gossipcache-net

networks:
  gossipcache-net:
    driver: bridge
```

### Dockerfile

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -tags example -o gossipcache ./examples/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /build/gossipcache .
COPY config/config.yaml .

EXPOSE 7946/tcp 7946/udp 8080 9090

ENTRYPOINT ["./gossipcache"]
CMD ["--config", "config.yaml"]
```

### Deployment

```bash
# Build image
docker build -t gossipcache:latest .

# Start cluster (backed mode)
docker-compose up -d

# Scale up
docker-compose up -d --scale cache=5

# View logs
docker-compose logs -f cache1

# Check health
curl http://localhost:9090/health
```

### Docker Swarm (Production)

```yaml
# docker-stack.yml
version: '3.8'

services:
  cache:
    image: gossipcache:latest
    deploy:
      replicas: 5
      update_config:
        parallelism: 1
        delay: 10s
      restart_policy:
        condition: on-failure
    environment:
      - MODE=backed
      - BACKING_STORE_TYPE=redis
      - BACKING_STORE_ADDRESS=redis:6379
      - DISCOVERY_MODE=docker_swarm
    ports:
      - "7946:7946"
      - "7946:7946/udp"
    networks:
      - gossipcache-net

  redis:
    image: redis:7-alpine
    deploy:
      placement:
        constraints:
          - node.role == manager
    networks:
      - gossipcache-net
    volumes:
      - redis-data:/data

networks:
  gossipcache-net:
    driver: overlay

volumes:
  redis-data:
```

```bash
# Deploy stack
docker stack deploy -c docker-stack.yml gossipcache

# Scale
docker service scale gossipcache_cache=10

# Remove
docker stack rm gossipcache
```

---

## Kubernetes Deployment

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                       │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐ │
│  │               Application Namespace                     │ │
│  │                                                          │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │           GossipCache StatefulSet                │  │ │
│  │  │  ┌────────┐  ┌────────┐  ┌────────┐             │  │ │
│  │  │  │ Pod 0  │  │ Pod 1  │  │ Pod 2  │             │  │ │
│  │  │  │ Cache  │  │ Cache  │  │ Cache  │             │  │ │
│  │  │  └───┬────┘  └───┬────┘  └───┬────┘             │  │ │
│  │  └──────┼───────────┼───────────┼──────────────────┘  │ │
│  │         │           │           │                      │ │
│  │         └───────────┼───────────┘                      │ │
│  │                     │                                  │ │
│  │         ┌───────────▼───────────┐                      │ │
│  │         │   Headless Service    │                      │ │
│  │         │  (Peer Discovery)     │                      │ │
│  │         └───────────────────────┘                      │ │
│  │                                                          │ │
│  │  ┌──────────────────────────────────────────────────┐  │ │
│  │  │         Redis/Postgres Service                   │  │ │
│  │  │         (Backed Mode)                            │  │ │
│  │  └──────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────┘ │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### Kubernetes Manifests

#### 1. Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: gossipcache
```

#### 2. ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gossipcache-config
  namespace: gossipcache
data:
  config.yaml: |
    mode: backed
    cache:
      max_size: 2GB
      default_ttl: 5m
      eviction_policy: lru

    gossip:
      interval: 1s
      fanout: 3
      anti_entropy_interval: 5m

    backing_store:
      type: redis
      address: redis-service:6379
      pool_size: 50

    discovery:
      mode: kubernetes
      kubernetes:
        namespace: gossipcache
        label_selector: app=gossipcache
        port_name: gossip

    network:
      tcp_port: 7946
      udp_port: 7946

    metrics:
      enabled: true
      port: 9090
```

#### 3. StatefulSet

```yaml
# statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: gossipcache
  namespace: gossipcache
spec:
  serviceName: gossipcache-headless
  replicas: 3
  selector:
    matchLabels:
      app: gossipcache
  template:
    metadata:
      labels:
        app: gossipcache
    spec:
      serviceAccountName: gossipcache
      containers:
      - name: gossipcache
        image: gossipcache:latest
        imagePullPolicy: Always
        ports:
        - name: gossip-tcp
          containerPort: 7946
          protocol: TCP
        - name: gossip-udp
          containerPort: 7946
          protocol: UDP
        - name: http
          containerPort: 8080
          protocol: TCP
        - name: metrics
          containerPort: 9090
          protocol: TCP
        env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: NODE_ID
          value: "$(POD_NAME)"
        volumeMounts:
        - name: config
          mountPath: /etc/gossipcache
          readOnly: true
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: 2000m
            memory: 4Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: gossipcache-config
```

#### 4. Headless Service (Peer Discovery)

```yaml
# service-headless.yaml
apiVersion: v1
kind: Service
metadata:
  name: gossipcache-headless
  namespace: gossipcache
spec:
  clusterIP: None
  selector:
    app: gossipcache
  ports:
  - name: gossip-tcp
    port: 7946
    targetPort: 7946
    protocol: TCP
  - name: gossip-udp
    port: 7946
    targetPort: 7946
    protocol: UDP
```

#### 5. Regular Service (Client Access)

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: gossipcache
  namespace: gossipcache
spec:
  type: ClusterIP
  selector:
    app: gossipcache
  ports:
  - name: http
    port: 8080
    targetPort: 8080
    protocol: TCP
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
```

#### 6. ServiceAccount & RBAC

```yaml
# rbac.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gossipcache
  namespace: gossipcache
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: gossipcache
  namespace: gossipcache
rules:
- apiGroups: [""]
  resources: ["pods", "endpoints"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: gossipcache
  namespace: gossipcache
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: gossipcache
subjects:
- kind: ServiceAccount
  name: gossipcache
  namespace: gossipcache
```

#### 7. Redis (Backed Mode)

```yaml
# redis.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: gossipcache
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        resources:
          requests:
            cpu: 100m
            memory: 512Mi
          limits:
            cpu: 500m
            memory: 2Gi
---
apiVersion: v1
kind: Service
metadata:
  name: redis-service
  namespace: gossipcache
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
```

### Deployment Steps

```bash
# Create namespace
kubectl apply -f namespace.yaml

# Create RBAC
kubectl apply -f rbac.yaml

# Create ConfigMap
kubectl apply -f configmap.yaml

# Deploy Redis (backed mode)
kubectl apply -f redis.yaml

# Deploy GossipCache
kubectl apply -f statefulset.yaml
kubectl apply -f service-headless.yaml
kubectl apply -f service.yaml

# Check status
kubectl get pods -n gossipcache
kubectl logs -n gossipcache gossipcache-0 -f

# Test
kubectl port-forward -n gossipcache svc/gossipcache 8080:8080
curl http://localhost:8080/health
```

### Scaling

```bash
# Scale up
kubectl scale statefulset gossipcache -n gossipcache --replicas=5

# Scale down (graceful)
kubectl scale statefulset gossipcache -n gossipcache --replicas=3

# Check gossip membership
kubectl exec -n gossipcache gossipcache-0 -- wget -qO- localhost:8080/api/v1/peers
```

### Helm Chart (Future)

```bash
# Install via Helm (coming soon)
helm repo add gossipcache https://charts.gossipcache.io
helm install my-cache gossipcache/gossipcache \
  --set mode=backed \
  --set redis.enabled=true \
  --set replicas=5
```

---

## Configuration Reference

### Environment Variables

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `NODE_ID` | Unique node identifier | hostname | No |
| `MODE` | Operating mode (backed/independent) | backed | Yes |
| `BACKING_STORE_TYPE` | Type of backing store | redis | No |
| `BACKING_STORE_ADDRESS` | Backing store address | localhost:6379 | No |
| `DISCOVERY_MODE` | Discovery mechanism | static | Yes |
| `GOSSIP_PEERS` | Static peer list | - | No |
| `MAX_CACHE_SIZE` | Max cache size | 1GB | No |
| `GOSSIP_INTERVAL` | Gossip frequency | 1s | No |
| `CONFLICT_STRATEGY` | Conflict resolution | last_write_wins | No |

### Full Configuration Example

See [TECHNICAL_SPEC.md](TECHNICAL_SPEC.md#34-configuration) for complete configuration schema.

---

## Monitoring & Operations

### Prometheus Metrics

```yaml
# ServiceMonitor (for Prometheus Operator)
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: gossipcache
  namespace: gossipcache
spec:
  selector:
    matchLabels:
      app: gossipcache
  endpoints:
  - port: metrics
    interval: 30s
```

### Grafana Dashboard

Import dashboard from `monitoring/grafana-dashboard.json` (coming soon).

**Key Metrics:**
- Cache hit/miss ratio
- Gossip message rate
- Peer count
- Memory usage
- Request latency

### Alerting Rules

```yaml
# alerts.yaml
groups:
- name: gossipcache
  interval: 30s
  rules:
  - alert: GossipCacheLowHitRate
    expr: rate(gossipcache_hits_total[5m]) / (rate(gossipcache_hits_total[5m]) + rate(gossipcache_misses_total[5m])) < 0.7
    for: 5m
    annotations:
      summary: "Low cache hit rate on {{ $labels.node_id }}"

  - alert: GossipCachePeerDown
    expr: gossipcache_peers{status="dead"} > 0
    for: 2m
    annotations:
      summary: "Dead peers detected in cluster"

  - alert: GossipCacheBackingStoreDown
    expr: gossipcache_backing_store_errors_total > 10
    for: 1m
    annotations:
      summary: "Backing store errors on {{ $labels.node_id }}"
```

---

## Troubleshooting

### Common Issues

#### 1. Nodes Not Discovering Each Other

**Symptoms**: Single peer count, no gossip traffic

**Solutions**:
- Check network connectivity (ping peers)
- Verify discovery configuration
- Check security groups/firewall rules
- Review logs for discovery errors

```bash
# Debug discovery
kubectl logs -n gossipcache gossipcache-0 | grep discovery

# Check peers
curl http://localhost:8080/api/v1/peers
```

#### 2. High Cache Miss Rate

**Symptoms**: Most requests go to backing store

**Solutions**:
- Increase cache size
- Tune TTL values
- Check eviction rate
- Review access patterns

#### 3. Gossip Storm

**Symptoms**: High network traffic, CPU usage

**Solutions**:
- Reduce gossip interval
- Lower fanout
- Check for loops in topology
- Review anti-entropy frequency

#### 4. Backing Store Connection Failures

**Symptoms**: Errors accessing Redis/Postgres

**Solutions**:
- Check backing store health
- Verify connection string
- Review network policies
- Check authentication credentials

### Debug Mode

```yaml
# Enable debug logging
logging:
  level: debug
  format: json
```

### Support

For issues and questions:
- GitHub Issues: https://github.com/sanketn26/gossipcache/issues
- Discussions: https://github.com/sanketn26/gossipcache/discussions
- Docs: https://docs.gossipcache.io
