import sqlite3, json
db = sqlite3.connect('<server-dir>/skillhub.db')
db.execute('PRAGMA busy_timeout=10000')
out = {}
# 24h 各机器 push/新建版本
out['versions_24h_by_machine'] = db.execute("""
    SELECT m.hostname, COUNT(*), SUM(CASE WHEN v.kind='merged' THEN 1 ELSE 0 END)
    FROM versions v JOIN skills s ON s.id=v.skill_id JOIN machines m ON m.id=s.machine_id
    WHERE v.created_at >= datetime('now', '-1 day') GROUP BY m.hostname""").fetchall()
out['machines'] = db.execute("SELECT hostname, push_count, last_seen FROM machines").fetchall()
# jobs
out['jobs_24h'] = db.execute("""
    SELECT status, COUNT(*) FROM jobs WHERE created_at >= datetime('now','-1 day') GROUP BY status""").fetchall()
out['jobs_total'] = db.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]
# 版本数 Top10（24h 内有活动的 skill，全历史版本数）
out['top_versions_24h'] = db.execute("""
    SELECT m.hostname, s.name, COUNT(*) AS c, MIN(v.created_at), MAX(v.created_at)
    FROM versions v JOIN skills s ON s.id=v.skill_id JOIN machines m ON m.id=s.machine_id
    WHERE v.created_at >= datetime('now','-1 day') AND v.kind='push'
    GROUP BY s.id ORDER BY c DESC LIMIT 12""").fetchall()
# 全历史版本 Top（含旧垃圾）
out['top_versions_all'] = db.execute("""
    SELECT m.hostname, s.name, COUNT(*) FROM versions v
    JOIN skills s ON s.id=v.skill_id JOIN machines m ON m.id=s.machine_id
    GROUP BY s.id ORDER BY c DESC LIMIT 8""", ()).fetchall() if False else db.execute("""
    SELECT m.hostname, s.name, COUNT(*) FROM versions v
    JOIN skills s ON s.id=v.skill_id JOIN machines m ON m.id=s.machine_id
    GROUP BY s.id ORDER BY COUNT(*) DESC LIMIT 8""").fetchall()
out['totals'] = db.execute("SELECT (SELECT COUNT(*) FROM skills),(SELECT COUNT(*) FROM versions),(SELECT COUNT(*) FROM machines)").fetchone()
out['dup_open'] = db.execute("SELECT COUNT(*) FROM dup_groups WHERE status='open'").fetchone()[0]
out['blob_bytes'] = db.execute("SELECT COALESCE(SUM(size),0) FROM blobs").fetchone()[0]
db.close()
print(json.dumps(out, ensure_ascii=False))
