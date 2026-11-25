interface IDisk {
  id: number;
  name: string;
  path: string;
  createdAt: string;
  virtual: boolean;
  online: boolean;
  total: number;
  used: number;
  free: number;
  userCount: number;
  safetyReserve: number;
}

interface IDiskCandidate {
  name: string;
  path: string;
  fstype: string;
  total: number;
  free: number;
}

interface IMoveJob {
  id: string;
  userID: number;
  username: string;
  fromDisk: number;
  toDisk: number;
  state: "running" | "done" | "failed";
  total: number;
  copied: number;
  error?: string;
  started: string;
  finished: string;
}

interface IUserUsage {
  id: number;
  used: number;
  quota: number;
  diskID: number;
  offline: boolean;
  unknown?: boolean;
}

interface IBrowseEntry {
  name: string;
  path: string;
  mount: boolean;
  readOnly: boolean;
  selectable: boolean;
  reason?: "root" | "server-root" | "not-mount" | "registered" | "nested";
  total?: number;
  free?: number;
}

interface IBrowseResult {
  path: string;
  parent: string;
  selectable: boolean;
  reason?: IBrowseEntry["reason"];
  entries: IBrowseEntry[];
}
