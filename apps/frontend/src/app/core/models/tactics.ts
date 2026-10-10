/** A tactics board as the API returns it. `content` is the board document. */
export interface TacticBoard {
  slug: string;
  owner_id: string;
  owner_name: string;
  title: string;
  description: string;
  content: unknown;
  created_at: string;
  updated_at: string;
}

/** A board in a list: no document, but how many steps it has. */
export interface TacticSummary {
  slug: string;
  owner_id: string;
  owner_name: string;
  title: string;
  description: string;
  steps: number;
  created_at: string;
  updated_at: string;
}
