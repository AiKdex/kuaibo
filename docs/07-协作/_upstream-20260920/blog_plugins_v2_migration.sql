-- blog_plugins v2 六列迁移（幂等：逐列 hasColumn 判据，与主系统 db.go 同款）
-- 独立执行或并入你方 repo/db.go Migrate 均可

ALTER TABLE blog_plugins ADD COLUMN kind TEXT NOT NULL DEFAULT 'ui';
ALTER TABLE blog_plugins ADD COLUMN capabilities TEXT NOT NULL DEFAULT '[]';
ALTER TABLE blog_plugins ADD COLUMN backend_entry TEXT NOT NULL DEFAULT '';
ALTER TABLE blog_plugins ADD COLUMN routes TEXT NOT NULL DEFAULT '[]';
ALTER TABLE blog_plugins ADD COLUMN min_schema INTEGER NOT NULL DEFAULT 0;
ALTER TABLE blog_plugins ADD COLUMN max_schema INTEGER;
ALTER TABLE blog_plugins ADD COLUMN hooks TEXT NOT NULL DEFAULT '{}';
-- 注：SQLite ADD COLUMN 不支持 IF NOT EXISTS，请按 hasColumn 判据逐列执行；
-- 参考判据：SELECT COUNT(*) FROM pragma_table_info('blog_plugins') WHERE name='capabilities'
