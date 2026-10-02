const API_URL = "http://localhost:6767";

export async function fetchToday() {
  const response = await fetch(`${API_URL}/today`);
  if (!response.ok) throw new Error("Could not load articles")
  return response.json();
}

export async function fetchInterests() {
  const response = await fetch(`${API_URL}/interests`);
  if (!response.ok) throw new Error("Could not load interests")
  return response.json();
}

export interface InterestUpdate {
  isActive: boolean;
  isMain: boolean;
}

export async function changeInterests(updates: Record<number, InterestUpdate>) {
  const response = await fetch(`${API_URL}/change-interests`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => null);
    throw new Error(typeof payload?.error === "string" ? payload.error : "Could not save interests");
  }
}
