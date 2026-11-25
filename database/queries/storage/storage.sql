-- name: InsertContent :exec
INSERT INTO content(kind,id,slug,color,published,created_at,payload) VALUES(?,?,?,?,?,?,?);

-- name: EnqueueBackup :exec
INSERT INTO backup_queue(operation,payload) VALUES(?,?);

-- name: CountBackups :one
SELECT count(*) FROM backup_queue;

-- name: NextBackup :one
SELECT sequence,operation,payload FROM backup_queue ORDER BY sequence LIMIT 1;

-- name: DeleteBackup :exec
DELETE FROM backup_queue WHERE sequence=?;

-- name: GetInitialized :one
SELECT value FROM settings WHERE key='initialized';

-- name: SetInitialized :exec
INSERT INTO settings(key,value) VALUES('initialized','1');
