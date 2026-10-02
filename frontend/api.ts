const API_URL = "http://localhost:6767";

export async function fetchToday() {
  const response = await fetch(`${API_URL}/today`);
  if (!response.ok) throw new Error("Could not load articles")
  return response.json();
}