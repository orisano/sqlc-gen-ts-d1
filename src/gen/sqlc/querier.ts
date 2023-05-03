import {D1Database, D1Result} from "@cloudflare/workers-types/2022-11-30"

const getAccountQuery = `-- name: GetAccount :one
SELECT pk, id, display_name, email FROM account WHERE id = ?1`;

export type GetAccountParams = {
  accountId: string;
};

export type GetAccountRow = {
  pk: bigint;
  id: string;
  display_name: string;
  email: string | null;
};

export async function getAccount(
  d1: D1Database,
  args: GetAccountParams
): Promise<GetAccountRow> {
  return await d1
    .prepare(getAccountQuery)
    .bind(args.accountId)
    .first<GetAccountRow>();
}

const listAccountsQuery = `-- name: ListAccounts :many
SELECT pk, id, display_name, email FROM account`;

export type ListAccountsParams = {
};

export type ListAccountsRow = {
  pk: bigint;
  id: string;
  display_name: string;
  email: string | null;
};

export async function listAccounts(
  d1: D1Database,
  args: ListAccountsParams
): Promise<D1Result<ListAccountsRow>> {
  return await d1
    .prepare(listAccountsQuery)
    .all<ListAccountsRow>();
}

const createAccountQuery = `-- name: CreateAccount :exec
INSERT INTO account (id, display_name, email)
VALUES (?1, ?2, ?3)`;

export type CreateAccountParams = {
  id: string;
  displayName: string;
  email: string;
};

export type CreateAccountRow = {
};

export async function createAccount(
  d1: D1Database,
  args: CreateAccountParams
): Promise<D1Result<CreateAccountRow>> {
  return await d1
    .prepare(createAccountQuery)
    .bind(args.id, args.displayName, args.email)
    .run<CreateAccountRow>();
}

const updateAccountDisplayNameQuery = `-- name: UpdateAccountDisplayName :one
UPDATE account
SET display_name = ?1
WHERE id = ?2
RETURNING pk, id, display_name, email`;

export type UpdateAccountDisplayNameParams = {
  displayName: string;
  id: string;
};

export type UpdateAccountDisplayNameRow = {
  pk: bigint;
  id: string;
  display_name: string;
  email: string | null;
};

export async function updateAccountDisplayName(
  d1: D1Database,
  args: UpdateAccountDisplayNameParams
): Promise<UpdateAccountDisplayNameRow> {
  return await d1
    .prepare(updateAccountDisplayNameQuery)
    .bind(args.displayName, args.id)
    .first<UpdateAccountDisplayNameRow>();
}

