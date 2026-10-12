# TODO

- [x] GKE（k8s）に CF Tunnel を入れる（2026-10-10）
  - cluster 内に cloudflared の Deployment を置いて `control:80` に繋ぐ。GCE Ingress（外部 LB・固定 IP `suyasuya-ip`・ManagedCertificate）を外す
  - 狙いは外部 LB の転送ルール 2 本分（約 $18/月）の削減
  - 審査員が使うので CF Access は掛けない（アプリのログインは残す）
  - DNS と Tunnel の route は Cloudflare の UI で設定する

- [x] 分析画面のグラフ：図形（棒・点）を選んだら、隣か空いている所に詳細を出す（2026-10-12）
  - 済：グラフ下の固定欄に出す。2 つのグラフで選択を揃える。起床時刻は analytics の `wake` を新設して出す（日別の表にも列を追加）
  - 今は `<title>` の tooltip だけ（`internal/web/sleep-charts.js`）。hover が無いスマホでは見えず、出るのも遅い
  - click / tap / Enter（`tabindex="0"` は既に付いている）で選択状態にし、詳細パネルに日付・主睡眠・昼寝・就寝/起床・不完全記録の有無を出す
  - 置き場所は図形の隣（はみ出すなら反対側）か、グラフ下の固定欄。スマホは固定欄の方が安定

- [x] agent ログ画面：task の区切りをはっきりさせる（2026-10-12）
  - 済：task ごとに枠、開いた task は枠と見出しの帯の色を変える、末尾の帯に「閉じる」（「◯◯ ここまで」の文字は冗長なので置かない）
  - 今は task（`details.agent-run`）の間が 1px の線だけで、複数開くとどの step がどの task のものか分からない（`internal/web/pages-agent.js`、`style.css` の agent log 節）
  - 案：task ごとに枠で囲む、開いた task は枠と見出しの帯の色を変える、step の末尾の帯に「閉じる」（「◯◯ ここまで」の文字は冗長なので置かない）を置く
