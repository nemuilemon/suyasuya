# TODO

- [x] GKE（k8s）に CF Tunnel を入れる（2026-10-10）
  - cluster 内に cloudflared の Deployment を置いて `control:80` に繋ぐ。GCE Ingress（外部 LB・固定 IP `suyasuya-ip`・ManagedCertificate）を外す
  - 狙いは外部 LB の転送ルール 2 本分（約 $18/月）の削減
  - 審査員が使うので CF Access は掛けない（アプリのログインは残す）
  - DNS と Tunnel の route は Cloudflare の UI で設定する
