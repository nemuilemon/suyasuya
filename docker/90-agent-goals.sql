BEGIN;
WITH retired AS (
 UPDATE agent_config_versions SET is_active=false WHERE config_key='job.plan_reminder' AND is_active AND body=$old$あなたは睡眠サポートagent「ありす」。設定時刻になったユーザーへ日本語で短く声をかける。
bedtime は今から作業を切り上げ、画面を閉じて寝る行動を促す。pvt は1分の反応時間テストを促す。
親しみがあり、少しきっぱりした声。説教・人格否定・罪悪感を煽る表現は使わない。
入力にない睡眠状態、予定、検査結果、過去の約束を作らない。返信や記録を要求しない。
通知1回分の本文のみを JSON の body に返す。1〜3文、300文字以内。URL・メンションは入れない。$old$ RETURNING *
 ) INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by)
 SELECT config_key,kind,(SELECT max(version)+1 FROM agent_config_versions WHERE config_key='job.plan_reminder'),$new$ユーザーが設定した時刻に合わせ、定時の就寝（bedtime）または1分PVTの実施（pvt）を支える。必要なデータをtoolで確認し、今伝える内容と送信の要否を判断する。$new$,true,created_by FROM retired;
WITH retired AS (
 UPDATE agent_config_versions SET is_active=false WHERE config_key='job.weekly_report' AND is_active AND body=$old$睡眠サポートagent「ありす」として、承認済み睡眠記録の週間集計を短く振り返る。
入力の欠測を睡眠ゼロとして扱わない。主睡眠と昼寝を区別する。
医療的な診断や因果の断定はしない。入力にない予定や事実を作らない。
数値の集計は別途表示するので、数値を再掲せず1〜3文、200文字以内で日本語の所感を返す。
記録がないときは評価せず、その旨だけ伝える。人格否定や記録を強いる表現は使わない。
JSON の comment に本文を返す。URL・メンションは使わない。$old$ RETURNING *
 ) INSERT INTO agent_config_versions(config_key,kind,version,body,is_active,created_by)
 SELECT config_key,kind,(SELECT max(version)+1 FROM agent_config_versions WHERE config_key='job.weekly_report'),$new$対象週の睡眠を振り返り、ユーザーが自分の睡眠の傾向を理解できるレポートを作る。承認済み記録のsnapshotはgetSleepTimeから取得できる。必要なtoolを使い、何を伝えるかと送信の要否を判断する。$new$,true,created_by FROM retired;
COMMIT;
