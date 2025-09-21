#!/bin/bash
set -e

echo "🔍 Verificando variáveis de ambiente..."
echo "POSTGRES_USER: $POSTGRES_USER"
echo "POSTGRES_DB: $POSTGRES_DB"
echo ""

# Configuração do usuário root
ROOT_USER="root"
ROOT_PASSWORD="root_password_123"

# Lista dos bancos de dados que você quer criar
databases=("identity" "catalog" "orders" "payments" "analytics")

echo "🔐 Criando usuário root com acesso a todos os bancos..."

# Criar usuário root com privilégios de superusuário
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$ROOT_USER') THEN
            CREATE USER $ROOT_USER WITH
                ENCRYPTED PASSWORD '$ROOT_PASSWORD'
                SUPERUSER
                CREATEDB
                CREATEROLE
                LOGIN;
            RAISE NOTICE 'Usuário root criado com privilégios de superusuário';
        ELSE
            RAISE NOTICE 'Usuário root já existe';
        END IF;
    END
    \$\$;
EOSQL

echo "✅ Usuário root criado!"
echo ""

# Configuração dos usuários (mesmo nome do banco + "_user")
for db in "${databases[@]}"; do
    db_user="${db}_user"
    db_pass="${db}_pass123"

    echo "Criando usuário: $db_user"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        DO \$\$
        BEGIN
            IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$db_user') THEN
                CREATE USER $db_user WITH ENCRYPTED PASSWORD '$db_pass';
                RAISE NOTICE 'Usuário $db_user criado';
            ELSE
                RAISE NOTICE 'Usuário $db_user já existe';
            END IF;
        END
        \$\$;
EOSQL

    echo "Criando banco de dados: $db"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        -- Criar banco se não existir
        SELECT 'CREATE DATABASE $db OWNER $db_user'
        WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$db')\gexec

        -- Conceder privilégios
        GRANT ALL PRIVILEGES ON DATABASE $db TO $POSTGRES_USER;
        GRANT ALL PRIVILEGES ON DATABASE $db TO $db_user;
        GRANT ALL PRIVILEGES ON DATABASE $db TO $ROOT_USER;
EOSQL

    echo "✅ Banco '$db' criado com usuário '$db_user'"
done

echo ""
echo "🎉 Todos os bancos de dados foram criados com sucesso!"
echo ""
echo "📋 INFORMAÇÕES DE CONEXÃO:"
echo "================================="
echo "🐘 USUÁRIO POSTGRES (Docker padrão):"
echo "   postgresql://postgres:postgres123@localhost:5432/[qualquer_banco]"
echo ""
echo "👑 USUÁRIO ROOT (acesso a todos):"
echo "   postgresql://$ROOT_USER:$ROOT_PASSWORD@localhost:5432/[qualquer_banco]"
echo ""
echo "🗄️  USUÁRIOS ESPECÍFICOS:"
for db in "${databases[@]}"; do
    db_user="${db}_user"
    db_pass="${db}_pass123"
    echo "   $db: postgresql://$db_user:$db_pass@localhost:5432/$db"
done
