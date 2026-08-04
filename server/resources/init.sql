CREATE DATABASE IF NOT EXISTS payment_db DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE payment_db;

CREATE TABLE IF NOT EXISTS items (
    id   BIGINT       NOT NULL AUTO_INCREMENT,
    name VARCHAR(128) NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_items_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_payment_account (
    id                   BIGINT       NOT NULL AUTO_INCREMENT,
    user_id              BIGINT       NOT NULL,
    provider             VARCHAR(32)  NOT NULL,
    provider_customer_id VARCHAR(128) NOT NULL,
    created_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_user_payment_account_user_id (user_id),
    UNIQUE KEY uk_provider_customer (provider, provider_customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_payment_method (
    id                         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id                    BIGINT       NOT NULL,
    provider                   VARCHAR(32)  NOT NULL,
    provider_customer_id       VARCHAR(128) NOT NULL DEFAULT '',
    provider_payment_method_id VARCHAR(128) NOT NULL,
    type                       VARCHAR(32)  NOT NULL DEFAULT '',
    brand                      VARCHAR(32)  NOT NULL DEFAULT '',
    last4                      VARCHAR(8)   NOT NULL DEFAULT '',
    status                     VARCHAR(16)  NOT NULL DEFAULT 'ACTIVE',
    created_at                 DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at                 DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_user_payment_method_user_id (user_id),
    KEY idx_user_payment_method_status (status),
    UNIQUE KEY uk_provider_pm (provider, provider_payment_method_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_payment_transaction (
    id                  BIGINT       NOT NULL AUTO_INCREMENT,
    payment_id          VARCHAR(16)  NOT NULL,
    biz_id              VARCHAR(128) NOT NULL,
    user_id             BIGINT       NOT NULL,
    payment_method_id   BIGINT       NOT NULL DEFAULT 0,
    amount              BIGINT       NOT NULL,
    currency            VARCHAR(16)  NOT NULL DEFAULT 'sgd',
    provider            VARCHAR(32)  NOT NULL,
    provider_payment_id VARCHAR(128) NOT NULL DEFAULT '',
    status              VARCHAR(32)  NOT NULL,
    remark              VARCHAR(512) NOT NULL DEFAULT '',
    created_at          DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at          DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_payment_id (payment_id),
    UNIQUE KEY uk_biz_id (biz_id),
    KEY idx_user_payment_transaction_user_id (user_id),
    KEY idx_user_payment_transaction_provider_payment_id (provider_payment_id),
    KEY idx_user_payment_transaction_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
