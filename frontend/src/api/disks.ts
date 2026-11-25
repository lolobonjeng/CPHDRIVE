import { fetchJSON, fetchURL } from "./utils";

export async function getAll(): Promise<IDisk[]> {
  return fetchJSON<IDisk[]>("/api/disks", {});
}

export async function create(name: string, path: string): Promise<IDisk> {
  const res = await fetchURL("/api/disks", {
    method: "POST",
    body: JSON.stringify({ name, path }),
  });
  return res.json();
}

export async function update(
  id: number,
  name: string,
  path: string
): Promise<IDisk> {
  const res = await fetchURL(`/api/disks/${id}`, {
    method: "PUT",
    body: JSON.stringify({ name, path }),
  });
  return res.json();
}

export async function remove(id: number): Promise<void> {
  await fetchURL(`/api/disks/${id}`, { method: "DELETE" });
}

export async function detect(): Promise<IDiskCandidate[]> {
  return fetchJSON<IDiskCandidate[]>("/api/disks/detect", {});
}

export async function moveUser(
  userID: number,
  diskID: number
): Promise<{ id: string }> {
  const res = await fetchURL(`/api/users/${userID}/move`, {
    method: "POST",
    body: JSON.stringify({ diskID }),
  });
  return res.json();
}

export async function getJob(id: string): Promise<IMoveJob> {
  return fetchJSON<IMoveJob>(`/api/jobs/${id}`, {});
}
