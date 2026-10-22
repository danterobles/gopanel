CREATE TABLE login_rate_limits (
  key_hash BINARY(32) NOT NULL PRIMARY KEY,
  window_start DATETIME(6) NOT NULL,
  attempts INT UNSIGNED NOT NULL DEFAULT 0,
  KEY ix_login_limits_window (window_start)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
