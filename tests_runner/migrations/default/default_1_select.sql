
create table contact_types
(
    id varchar(50) not null
        constraint contact_types_pkey
            primary key,
    created_at timestamp with time zone,
    modified_at timestamp with time zone,
    version bigint,
    tenant_id varchar(50),
    partition_id varchar(50),
    deleted_at timestamp with time zone,
    uid bigint,
    name text,
    description text
);


SELECT 1;