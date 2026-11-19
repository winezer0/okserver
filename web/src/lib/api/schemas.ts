import { z } from "zod";

export const uploadSchema = z.object({
  id: z.string(),
  path: z.string(),
  size: z.number().nonnegative(),
  offset: z.number().nonnegative(),
  checksum_algorithm: z.string(),
  checksum: z.string().optional(),
  chunk_size: z.number().positive(),
  completed: z.boolean().optional(),
  updated_at: z.string().optional(),
});
export type Upload = z.infer<typeof uploadSchema>;
export const tasksSchema = z.object({
  uploads: z
    .array(uploadSchema)
    .nullable()
    .transform((value) => value || []),
});
const entrySchema = z.object({
  name: z.string(),
  path: z.string(),
  is_dir: z.boolean(),
  size: z.number(),
  modified: z.string(),
});
export const directorySchema = z.object({
  path: z.string(),
  entries: z.array(entrySchema),
  page: z.number().int().positive(),
  limit: z.number().int().positive(),
  total: z.number().nonnegative(),
  files: z.number(),
  folders: z.number(),
  bytes: z.number(),
});
export type FileEntry = z.infer<typeof entrySchema>;
export type Directory = z.infer<typeof directorySchema>;
export const capabilitiesSchema = z.object({
  allow_create_dir: z.boolean(),
  allow_delete_file: z.boolean(),
});
