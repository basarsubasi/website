export interface PhotoInput {
  color: "bw" | "color";
  name: string;
  country: string;
  city: string;
  year_taken: number;
}
export interface Photo extends PhotoInput {
  uuid: string;
  image_url: string;
  object_key: string;
  file_type: string;
  created_at: string;
}
export interface Post {
  id: string;
  slug: string;
  title: string;
  excerpt: string;
  body: string;
  published: 0 | 1;
  created_at: string;
  updated_at: string;
}
export type PostInput = Omit<Post, "id" | "created_at" | "updated_at">;
export interface ListResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
}
export interface ItemResponse<T> {
  data: T;
}
export interface Session {
  authenticated: boolean;
}
