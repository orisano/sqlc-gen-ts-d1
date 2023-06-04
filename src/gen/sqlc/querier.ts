import { Account } from "./models"

const getAccountQuery = `-- name: GetAccount :one
SELECT pk, id, display_name, email FROM account WHERE id = ?1`;

export type GetAccountParams = {
  accountId: string;
};

export type GetAccountRow = {
  pk: number;
  id: string;
  displayName: string;
  email: string | null;
};

type RawGetAccountRow = {
  pk: number;
  id: string;
  display_name: string;
  email: string | null;
};

export async function getAccount(
  db: any,
  args: GetAccountParams
): Promise<GetAccountRow | null> {
  return Promise.resolve(
    db.selectObject(getAccountQuery, [args.accountId]) || null
  )
  .then(raw => raw ? {
    pk: raw.pk,
    id: raw.id,
    displayName: raw.display_name,
    email: raw.email,
  } : null) as Promise<GetAccountRow | null>;
}

const listAccountsQuery = `-- name: ListAccounts :many
SELECT account.pk AS account_pk, account.id AS account_id, account.display_name AS account_display_name, account.email AS account_email FROM account`;

export type ListAccountsRow = {
  account: Account;
};

type RawListAccountsRow = {
  account_pk: number;
  account_id: string;
  account_display_name: string;
  account_email: string | null;
};

export async function listAccounts(
  db: any
): Promise<ListAccountsRow[]> {
  return Promise.resolve(
    db.selectObjects(listAccountsQuery, [])
  )
  .then(raws => raws.map(raw => { return {
    account: {
      pk: raw.account_pk,
      id: raw.account_id,
      displayName: raw.account_display_name,
      email: raw.account_email,
    },
  }})) as Promise<ListAccountsRow[]>;
}

const createAccountQuery = `-- name: CreateAccount :exec
INSERT INTO account (id, display_name, email)
VALUES (?1, ?2, ?3)`;

export type CreateAccountParams = {
  id: string;
  displayName: string;
  email: string | null;
};

export async function createAccount(
  db: any,
  args: CreateAccountParams
): Promise<any> {
  return Promise.resolve(
    db.exec(createAccountQuery, [args.id, args.displayName, args.email])
  ) as Promise<any>;
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
  pk: number;
  id: string;
  displayName: string;
  email: string | null;
};

type RawUpdateAccountDisplayNameRow = {
  pk: number;
  id: string;
  display_name: string;
  email: string | null;
};

export async function updateAccountDisplayName(
  db: any,
  args: UpdateAccountDisplayNameParams
): Promise<UpdateAccountDisplayNameRow | null> {
  return Promise.resolve(
    db.selectObject(updateAccountDisplayNameQuery, [args.displayName, args.id]) || null
  )
  .then(raw => raw ? {
    pk: raw.pk,
    id: raw.id,
    displayName: raw.display_name,
    email: raw.email,
  } : null) as Promise<UpdateAccountDisplayNameRow | null>;
}

const getAccountsQuery = `-- name: GetAccounts :many
SELECT pk, id, display_name, email FROM account WHERE id IN (/*SLICE:ids*/?)`;

export type GetAccountsParams = {
  ids: string[];
};

export type GetAccountsRow = {
  pk: number;
  id: string;
  displayName: string;
  email: string | null;
};

type RawGetAccountsRow = {
  pk: number;
  id: string;
  display_name: string;
  email: string | null;
};

export async function getAccounts(
  db: any,
  args: GetAccountsParams
): Promise<GetAccountsRow[]> {
  let query = getAccountsQuery;
  const params: any[] = [args.ids[0]];
  query = query.replace("(/*SLICE:ids*/?)", expandedParam(1, args.ids.length, params.length));
  params.push(...args.ids.slice(1));
  return Promise.resolve(
    db.selectObjects(query, [args.ids[0]])
  )
  .then(raws => raws.map(raw => { return {
    pk: raw.pk,
    id: raw.id,
    displayName: raw.display_name,
    email: raw.email,
  }})) as Promise<GetAccountsRow[]>;
}

function expandedParam(n: number, len: number, last: number): string {
  const params: number[] = [n];
  for (let i = 1; i < len; i++) {
    params.push(last + i);
  }
  return "(" + params.map((x: number) => "?" + x).join(", ") + ")";
}
